package pipeline

import (
	"fmt"
	"log"
	"time"

	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/types"
)

type smEventType int

const (
	evNone smEventType = iota
	eventPipelineAdd
	eventPipelineRun
	eventPipelineStop
	eventPipelineStatus
	eventInstanceDelete
	eventTaskCreate
	eventTaskAbort
	eventTaskComplete
)

type smEvent interface {
	eventType() smEventType
	String() string
}

type evPipelineAdd struct {
	pipeline *Pipeline
}

func (ev *evPipelineAdd) eventType() smEventType { return eventPipelineAdd }
func (ev *evPipelineAdd) String() string         { return "ADD " + ev.pipeline.Name }
func (exec *mrExecutor) handlePipelineAdd(event *evPipelineAdd) {
	p := event.pipeline
	switch p.State {
	case "":
		p.State = StateStopped
	case StateRunning:
		// The job was previously running
	}
}

type evPipelineRun struct {
	pipeline   *Pipeline
	instanceID int
	taskIndex  int
}

func (ev *evPipelineRun) eventType() smEventType { return eventPipelineRun }
func (ev *evPipelineRun) String() string {
	return fmt.Sprintf("RUN %s:%d", ev.pipeline.Name, ev.instanceID)
}
func (exec *mrExecutor) handlePipelineRun(event *evPipelineRun) {
	p := event.pipeline
	instance := p.getInstance(event.instanceID)

	// delete all jobs greater >= taskIndex
	for i := event.taskIndex; i < len(instance.TaskList); i++ {
		p.deleteTaskResources(exec.k8sClient, instance, i)
	}

	p.State = StateRunning
	instance.Stage = event.taskIndex
	instance.State = StateRunning

	watch := MakeWatcher(p, instance)
	instance.watcher = watch
	go watch.Run(exec.k8sClient, exec.events)

	// create task
	exec.events <- &evTaskCreate{p, instance.ID, event.taskIndex}
}

type evPipelineStatus struct {
	pipeline *Pipeline
	instance *Instance
	jobID    types.UID
	status   batch_v1.JobStatus
}

func (ev *evPipelineStatus) eventType() smEventType { return eventPipelineStatus }
func (ev *evPipelineStatus) String() string {
	return "PipelineStatus " + ev.pipeline.Name
}
func (exec *mrExecutor) handlePipelineStatus(event *evPipelineStatus) {
	pipeline := event.pipeline
	instance := event.instance

	// fetch the running task
	task := instance.TaskList[instance.Stage]

	// ensure that this job is in the running task
	var jobName string
	for k, id := range task.JobIDs {
		if event.jobID == id {
			jobName = k
			break
		}
	}

	if jobName == "" {
		log.Printf("unexpected event for job %s", event.jobID)
		return
	}

	status := event.status
	var complete bool
	for _, cond := range status.Conditions {
		if cond.Type == batch_v1.JobComplete {
			complete = true
			break
		}
	}

	// if all jobs are complete, advance to next task
	if complete {
		log.Printf("job %s complete", event.jobID)
		task.completed++
	}

	if task.completed == len(task.jobs) {
		exec.events <- &evTaskComplete{
			pipeline:   pipeline,
			instanceID: instance.ID,
			taskIndex:  instance.Stage,
		}
		return
	}

	jcfg := task.getJobByName(jobName)
	if jcfg == nil {
		log.Printf("unknown job %s", jobName)
		return
	}
	var threshold int32
	if jcfg.Spec.Completions != nil {
		threshold = *jcfg.Spec.Completions
	}
	if threshold < 4 {
		threshold = 4
	}

	// if the number of failures has gone past threshold abort
	if status.Failed > threshold {
		exec.events <- &evTaskAbort{pipeline, instance.ID, instance.Stage, "Too many failures", time.Now()}
	}
}

type evPipelineStop struct {
	pipeline *Pipeline
}

func (ev *evPipelineStop) eventType() smEventType { return eventPipelineStop }
func (ev *evPipelineStop) String() string {
	return "PipelineStop " + ev.pipeline.Name
}
func (exec *mrExecutor) handlePipelineStop(event *evPipelineStop) {
	p := event.pipeline
	p.State = StateStopped
}

type evInstanceDelete struct {
	pipeline   *Pipeline
	instanceID int
}

func (ev *evInstanceDelete) eventType() smEventType { return eventInstanceDelete }
func (ev *evInstanceDelete) String() string {
	return fmt.Sprintf("DELETE %s:%d", ev.pipeline.Name, ev.instanceID)
}
func (exec *mrExecutor) handleInstanceDelete(event *evInstanceDelete) {
	p := event.pipeline
	instance := p.getInstance(event.instanceID)
	if instance != nil {
		p.deleteInstance(instance)
	}
}

func (exec *mrExecutor) instanceStop(p *Pipeline, instance *Instance) {
	instance.State = StateStopped
	instance.watcher.Shutdown()
	instance.watcher = nil

	var running int
	for _, instanceIter := range p.Instances {
		if instanceIter.State == StateRunning {
			running++
		}
	}
	if running == 0 {
		exec.events <- &evPipelineStop{p}
	}

}

type evTaskCreate struct {
	pipeline   *Pipeline
	instanceID int
	taskIndex  int
}

func (ev *evTaskCreate) eventType() smEventType { return eventTaskCreate }
func (ev *evTaskCreate) String() string {
	return fmt.Sprintf("TASK CREATE %s:%d", ev.pipeline.Name, ev.instanceID)
}
func (exec *mrExecutor) handleTaskCreate(event *evTaskCreate) {
	pipeline := event.pipeline
	instance := pipeline.getInstance(event.instanceID)

	task := pipeline.Config.Spec.Tasks[event.taskIndex]
	if task.EtcdLock != "" {
		// if err := etcdDeleteLock(pipeline.Spec.Namespace, pipeline.Spec.Name+"-"+task.EtcdLock, event.instanceID); err != nil {
		// 	glog.Error(err)
		// }
	}

	if len(task.Services) > 0 {
		pipeline.createServices(instance, event.taskIndex)
	}

	pipeline.createTask(exec.k8sClient, instance, event.taskIndex)
}

type evTaskAbort struct {
	pipeline     *Pipeline
	instanceID   int
	taskIndex    int
	msg          string
	transitionTs time.Time
}

func (ev *evTaskAbort) eventType() smEventType { return eventTaskAbort }
func (ev *evTaskAbort) String() string {
	return fmt.Sprintf("TASK ABORT %s:%d task:%d %s", ev.pipeline.Name, ev.instanceID, ev.taskIndex, ev.msg)
}
func (ev *evTaskAbort) transitionTime() time.Time { return ev.transitionTs }

func (exec *mrExecutor) handleTaskAbort(event *evTaskAbort) {
	p := event.pipeline
	instance := p.getInstance(event.instanceID)
	if instance == nil {
		log.Printf("%s unknown instance: %d", p.Name, event.instanceID)
		return
	}

	p.cancelInstance(exec.k8sClient, instance)
	exec.instanceStop(p, instance)
}

type evTaskComplete struct {
	pipeline     *Pipeline
	instanceID   int
	taskIndex    int
	transitionTs time.Time
}

func (ev *evTaskComplete) eventType() smEventType { return eventTaskComplete }
func (ev *evTaskComplete) String() string {
	return fmt.Sprintf("TASK COMPLETE %s:%d task: %d", ev.pipeline.Name, ev.instanceID, ev.taskIndex)
}
func (ev *evTaskComplete) transitionTime() time.Time { return ev.transitionTs }

func (exec *mrExecutor) handleTaskComplete(event *evTaskComplete) {
	p := event.pipeline
	instance := p.getInstance(event.instanceID)
	if instance == nil {
		log.Println("Invalid instance id ", event.instanceID)
		return
	}

	if event.taskIndex < len(p.Config.Spec.Tasks)-1 {
		instance.Stage = event.taskIndex + 1
		exec.events <- &evTaskCreate{p, event.instanceID, instance.Stage}
		return
	}

	// Instance Complete
	exec.instanceStop(p, instance)
}

func (exec *mrExecutor) periodicCheck() {
	exec.Lock()
	defer exec.Unlock()
}

func (exec *mrExecutor) runOnce(t *time.Ticker) {
	select {
	case ev := <-exec.events:
		// glog.V(1).Info(ev.String())
		log.Println(ev.String())
		switch ev.eventType() {
		case eventPipelineAdd:
			exec.handlePipelineAdd(ev.(*evPipelineAdd))
		case eventPipelineRun:
			exec.handlePipelineRun(ev.(*evPipelineRun))
		case eventPipelineStatus:
			exec.handlePipelineStatus(ev.(*evPipelineStatus))
		case eventPipelineStop:
			exec.handlePipelineStop(ev.(*evPipelineStop))
		case eventInstanceDelete:
			exec.handleInstanceDelete(ev.(*evInstanceDelete))
		case eventTaskCreate:
			exec.handleTaskCreate(ev.(*evTaskCreate))
		case eventTaskAbort:
			exec.handleTaskAbort(ev.(*evTaskAbort))
		case eventTaskComplete:
			exec.handleTaskComplete(ev.(*evTaskComplete))

		}

	case <-t.C:
		exec.periodicCheck()
		if exec.checkpointFile != "" {
			exec.checkpointConfig()
		}
	}
}

func (exec *mrExecutor) run() {
	t := time.NewTicker(time.Minute)
	for {
		exec.runOnce(t)
	}
}
