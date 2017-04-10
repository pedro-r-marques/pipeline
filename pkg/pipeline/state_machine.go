package pipeline

import (
	"fmt"
	"log"
	"time"
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
		exec.events <- &evPipelineStatus{p}
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

	// TODO: delete all jobs greater >= taskIndex
	p.deleteInstanceResource(instance, "job", event.taskIndex)
	p.deleteInstanceResource(instance, "svc", event.taskIndex)

	p.State = StateRunning
	instance.Stage = event.taskIndex
	instance.State = StateRunning
	// instance.JobsStatus = nil

	// create task
	exec.events <- &evTaskCreate{p, instance.ID, event.taskIndex}
}

type evPipelineStatus struct {
	pipeline *Pipeline
}

func (ev *evPipelineStatus) eventType() smEventType { return eventPipelineStatus }
func (ev *evPipelineStatus) String() string {
	return "PipelineStatus " + ev.pipeline.Name
}
func (exec *mrExecutor) handlePipelineStatus(event *evPipelineStatus) {
	p := event.pipeline
	var runCount int
	for _, instance := range p.Instances {
		if exec.updateInstanceStatus(p, instance) {
			runCount++
		}
	}

	if runCount > 0 {
		p.State = StateRunning
	} else {
		p.State = StateStopped
	}
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

	task := pipeline.Spec.Tasks[event.taskIndex]
	if task.EtcdLock != "" {
		// if err := etcdDeleteLock(pipeline.Spec.Namespace, pipeline.Spec.Name+"-"+task.EtcdLock, event.instanceID); err != nil {
		// 	glog.Error(err)
		// }
	}

	if len(task.Services) > 0 {
		pipeline.createServices(instance, event.taskIndex)
	}

	pipeline.createTask(instance, event.taskIndex)
	exec.updateInstanceStatus(pipeline, instance)
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
	instance := event.pipeline.getInstance(event.instanceID)
	if instance != nil {
		event.pipeline.stopInstance(instance)
		instance.State = StateStopped
	}
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

	if event.taskIndex < len(p.Spec.Tasks)-1 {
		instance.Stage = event.taskIndex + 1
		exec.events <- &evTaskCreate{p, event.instanceID, instance.Stage}
	} else {
		instance.State = StateStopped
	}
}

const instanceFailMinAbort = 4

func instanceFailuresShouldAbort(spec *Spec, taskIndex int, status []*jobStatus) bool {
	var current *jobStatus
	for _, s := range status {
		if s.taskIndex == taskIndex {
			current = s
		}
	}
	if current == nil {
		// glog.V(2).Infof("No current status for %s task %d", spec.Name, taskIndex)
		return false
	}
	if current.Active == 0 {
		return false
	}
	if current.Failed < instanceFailMinAbort {
		return false
	}
	task := spec.Tasks[taskIndex]
	var instanceCount int
	if len(task.TemplateList) > 0 {
		for i := range task.TemplateList {
			tmpl := task.TemplateList[i]
			instanceCount += tmpl.Instances
		}
	} else {
		instanceCount = task.Instances
	}

	return int(current.Failed) > (instanceCount + instanceCount/2)
}

func (exec *mrExecutor) updateInstanceStatus(p *Pipeline, instance *Instance) bool {
	// status := instanceJobStatus(p, instance)
	// events := jobStatusDelta(p, instance.ID, instance.JobsStatus, status)
	// instance.JobsStatus = status
	// nextStatus := taskState(status)
	// if instanceFailuresShouldAbort(p.Spec, instance.Stage, status) {
	// 	exec.events <- &evTaskAbort{p, instance.ID, instance.Stage, "Too many failures", time.Now()}
	// 	instance.Current = nextStatus
	// 	return true
	// }
	// if events != nil && len(events) > 0 {
	// 	lastEv := events[0]
	// 	exec.events <- lastEv
	// } else if instance.Current != nil &&
	// 	instance.Current.Running == 0 && nextStatus.Running == 0 {
	// 	instance.State = StateStopped
	// }
	// instance.Current = nextStatus

	return instance.State == StateRunning
}

func (exec *mrExecutor) periodicCheck() {
	exec.Lock()
	defer exec.Unlock()

	for _, j := range exec.pipelines {
		if j.State == StateRunning {
			exec.events <- &evPipelineStatus{j}
		}
	}
}

func (exec *mrExecutor) run() {
	t := time.NewTicker(time.Minute)
	for {
		select {
		case ev := <-exec.events:
			// glog.V(1).Info(ev.String())
			switch ev.eventType() {
			case eventPipelineAdd:
				exec.handlePipelineAdd(ev.(*evPipelineAdd))
			case eventPipelineRun:
				exec.handlePipelineRun(ev.(*evPipelineRun))
			case eventPipelineStatus:
				exec.handlePipelineStatus(ev.(*evPipelineStatus))
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
}
