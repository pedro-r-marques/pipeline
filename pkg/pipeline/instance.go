package pipeline

import (
	"encoding/json"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"time"

	batchapi "k8s.io/client-go/pkg/apis/batch/v1"
)

// An Instance keeps track of the processing steps related to a dataset.
type Instance struct {
	ID int

	// StartStage is stage that the pipeline was (re)started at.
	StartStage int

	workDir    string
	JobsStatus []*jobStatus

	Stage int
	State ExecState

	// Result of current status
	Current *taskStatus
}

type taskStatus struct {
	Running int
	Success int
	Failed  int
}

type jobStatus struct {
	JobName   string
	taskIndex int
	jobIndex  int
	batchapi.JobStatus
}

func (s *jobStatus) IsRunning() bool {
	return s.Active > 0
}

func (s *jobStatus) IsComplete() bool {
	if len(s.Conditions) > 0 {
		cond := &s.Conditions[0]
		return cond.Type == batchapi.JobComplete
	}
	return false
}

func (s *jobStatus) LastTransitionTime() time.Time {
	if len(s.Conditions) > 0 {
		return s.Conditions[0].LastTransitionTime.Time
	}
	return time.Time{}
}

func jobStatusCompare(lhs, rhs *jobStatus) int {
	if lhs.taskIndex < rhs.taskIndex {
		return -1
	} else if lhs.taskIndex > rhs.taskIndex {
		return 1
	}
	if lhs.jobIndex < rhs.jobIndex {
		return -1
	} else if lhs.jobIndex > rhs.jobIndex {
		return 1
	}

	return 0
}

type jobStatusByIndex []*jobStatus

func (s jobStatusByIndex) Len() int {
	return len(s)
}
func (s jobStatusByIndex) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s jobStatusByIndex) Less(i, j int) bool {
	return jobStatusCompare(s[i], s[j]) < 0
}

// instanceJobStatus check the status of a running instance.
func instanceJobStatus(p *Pipeline, instance *Instance) []*jobStatus {
	args := []string{
		"--namespace=" + p.Spec.Namespace,
		"get", "jobs",
		"-l", "pipeline=" + p.Spec.Name + ",id=" + strconv.Itoa(instance.ID),
		"-o", "json",
	}
	cmd := exec.Command("kubectl", args...)
	result, err := cmd.Output()
	if err != nil {
		log.Println(err)
		return nil
	}

	var jobList batchapi.JobList
	err = json.Unmarshal(result, &jobList)
	if err != nil {
		log.Println(err)
		return nil
	}

	var statusVec []*jobStatus

	// Determine whether there are running tasks and update the status appropriatly.
	for _, item := range jobList.Items {
		// glog.V(3).Info(item.Name, item.Status)
		taskIx, jobIx := p.getStageIndex(item.Labels["task"])
		if taskIx == -1 {
			// glog.Errorf("task %s %+v: not found", item.Name, item.Labels)
			continue
		}

		status := &jobStatus{
			item.Name,
			taskIx,
			jobIx,
			item.Status,
		}

		statusVec = append(statusVec, status)
	}

	// sort statusVec by index
	sort.Sort(jobStatusByIndex(statusVec))
	return statusVec
}

type taskEvent interface {
	smEvent
	transitionTime() time.Time
}

type taskEventsByTime []taskEvent

func (s taskEventsByTime) Len() int {
	return len(s)
}
func (s taskEventsByTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s taskEventsByTime) Less(i, j int) bool {
	return s[i].transitionTime().Before(s[j].transitionTime())
}

func jobListCompare(current, next []*jobStatus, add, remove func(*jobStatus), compare func(a, b *jobStatus)) {
	var i, j int
	for i < len(current) && j < len(next) {
		currStatus := current[i]
		nextStatus := next[j]
		if cmp := jobStatusCompare(currStatus, nextStatus); cmp < 0 {
			remove(currStatus)
			i++
		} else if cmp > 0 {
			add(nextStatus)
			j++
		} else {
			compare(currStatus, nextStatus)
			i++
			j++
		}
	}
	for ; i < len(current); i++ {
		remove(current[i])
	}
	for ; j < len(next); j++ {
		add(next[j])
	}

}

// jobStatusDelta compares the output of the k8s status in order to determine
// when a specific task completes.
// Several cases here:
// - task single k8s job; the job can be still running or have completed.
// - TODO: if the k8s job is trashing (status failing count incrementing) stop it
// - task with multiple jobs;
// - jobs could have been deleted via the k8s API.
func jobStatusDelta(p *Pipeline, id int, current, next []*jobStatus) []taskEvent {
	var eventList []taskEvent

	completeCount := make(map[int]int)
	completeTime := make(map[int]time.Time)

	jobCheckStatus := func(status *jobStatus) {
		if !status.IsRunning() {
			if status.IsComplete() {
				completeCount[status.taskIndex]++
				ts := status.LastTransitionTime()
				if lastTs := completeTime[status.taskIndex]; ts.After(lastTs) {
					completeTime[status.taskIndex] = ts
				}
			} else if len(status.Conditions) > 0 || status.Failed > 0 {
				// glog.V(2).Info(status)
				eventList = append(eventList, &evTaskAbort{
					p,
					id,
					status.taskIndex,
					status.JobName,
					status.LastTransitionTime(),
				})
			}
		}
	}

	jobListCompare(current, next,
		func(nextStatus *jobStatus) {
			// glog.V(2).Infof("job %s was added", nextStatus.JobName)
			jobCheckStatus(nextStatus)
		},
		func(currStatus *jobStatus) {
			if currStatus.IsRunning() {
				// glog.V(2).Infof("running job %s was deleted", currStatus.JobName)
			}
		},
		func(currStatus, nextStatus *jobStatus) {
			if currStatus.IsRunning() && !nextStatus.IsRunning() {
				// glog.V(2).Infof("job %s status changed", currStatus.JobName)
				jobCheckStatus(nextStatus)
			}
		},
	)

	instance := p.getInstance(id)
	if instance.Stage >= len(p.Spec.Tasks) {
		return []taskEvent{
			&evTaskAbort{p, id, instance.Stage, "", time.Now()},
		}
	}
	task := p.Spec.Tasks[instance.Stage]
	var completeNeeded int
	if len(task.TemplateList) > 0 {
		completeNeeded = len(task.TemplateList)
	} else {
		completeNeeded = 1
	}
	if completeCount[instance.Stage] == completeNeeded {
		eventList = append(eventList, &evTaskComplete{
			p, id, instance.Stage, completeTime[instance.Stage],
		})
	}

	// returns events sorted: most recent first
	if eventList != nil {
		sort.Sort(sort.Reverse(taskEventsByTime(eventList)))
	}
	return eventList
}

func taskState(statusVec []*jobStatus) *taskStatus {
	var running, success, failed int
	for _, status := range statusVec {
		running += int(status.Active)
		success += int(status.Succeeded)
		failed += int(status.Failed)
	}
	return &taskStatus{
		Running: running,
		Success: success,
		Failed:  failed,
	}
}
