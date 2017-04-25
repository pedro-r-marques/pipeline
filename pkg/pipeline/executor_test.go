package pipeline

import (
	"log"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
)

func genJobCompletionEvent(exec *mrExecutor, pipeline *Pipeline, job *batch_v1.Job) {
	exec.events <- &evPipelineStatus{
		pipeline: pipeline,
		instance: pipeline.Instances[0],
		jobID:    job.UID,
		status: batch_v1.JobStatus{
			Conditions: []batch_v1.JobCondition{
				{
					Type: batch_v1.JobComplete,
				},
			},
		},
	}
}

func TestPipelineExec(t *testing.T) {
	exec := &mrExecutor{
		pipelines: make(map[string]*Pipeline),
		dataDir:   "testdata",
		events:    make(chan smEvent, 16),
		k8sClient: fake.NewSimpleClientset(),
	}
	config := &Config{
		Spec: &Spec{
			Name:      "test",
			Namespace: "roque",
			Storage:   "gs://laserlike_roque/test",
			Tasks: []TaskSpec{
				{
					Name: "step1",
					JobTemplate: JobTemplate{
						Image:       "step1",
						Instances:   1,
						Parallelism: 1,
					},
				},
				{
					Name: "step2",
					TemplateList: []*JobTemplate{
						&JobTemplate{
							Job:         "2a",
							Image:       "step2a",
							Instances:   4,
							Parallelism: 4,
						},
						&JobTemplate{
							Job:         "2b",
							Image:       "step2b",
							Instances:   4,
							Parallelism: 2,
						},
					},
				},
			},
		},
	}

	defaultPipelineSpecValues(config.Spec, "../../templates")
	pipeline := &Pipeline{
		Name:   "test",
		State:  StateStopped,
		Config: config,
	}
	exec.pipelines[pipeline.Name] = pipeline

	exec.SetState(pipeline, ActionStart, 0, 0)

	timeout := time.NewTicker(time.Second)
	exec.runOnce(timeout)

	if pipeline.State != StateRunning {
		t.Error(pipeline.State)
	}

	exec.runOnce(timeout)
	jobList, err := exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobList.Items) != 1 {
		t.Error(len(jobList.Items))
	}

	genJobCompletionEvent(exec, pipeline, &jobList.Items[0])

	exec.runOnce(timeout)
	exec.runOnce(timeout)
	exec.runOnce(timeout)

	j2List, err := exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(j2List.Items) != 3 {
		t.Error(len(j2List.Items))
	}

	var j2a bool
	for _, job := range j2List.Items {
		log.Println(job.Name)
		if job.Name == "test-2a-1" {
			genJobCompletionEvent(exec, pipeline, &job)
			j2a = true
			break
		}
	}
	if !j2a {
		t.Error("job not found")
	}

	exec.runOnce(timeout)

	var j2b bool
	for _, job := range j2List.Items {
		if job.Name == "test-2b-1" {
			genJobCompletionEvent(exec, pipeline, &job)
			j2b = true
			break
		}
	}
	if !j2b {
		t.Error("job not found")
	}

	exec.runOnce(timeout)
	exec.runOnce(timeout)
	exec.runOnce(timeout)
	exec.runOnce(timeout)

	if pipeline.State != StateStopped {
		t.Error(pipeline.State)
	}
}

func TestTaskAbort(t *testing.T) {
	exec := &mrExecutor{
		pipelines: make(map[string]*Pipeline),
		dataDir:   "testdata",
		events:    make(chan smEvent, 16),
		k8sClient: fake.NewSimpleClientset(),
	}

	config := &Config{
		Spec: &Spec{
			Name:      "test",
			Namespace: "roque",
			Storage:   "gs://laserlike_roque/test",
			Tasks: []TaskSpec{
				{
					Name: "step1",
					JobTemplate: JobTemplate{
						Image:       "step1",
						Instances:   4,
						Parallelism: 2,
					},
				},
			},
		},
	}

	defaultPipelineSpecValues(config.Spec, "../../templates")
	pipeline := &Pipeline{
		Name:   "test",
		State:  StateStopped,
		Config: config,
	}
	exec.pipelines[pipeline.Name] = pipeline

	exec.SetState(pipeline, ActionStart, 0, 0)

	timeout := time.NewTicker(time.Second)
	exec.runOnce(timeout)

	if pipeline.State != StateRunning {
		t.Error(pipeline.State)
	}

	exec.runOnce(timeout)
	jobList, err := exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobList.Items) != 1 {
		t.Error(len(jobList.Items))
	}

	job := &jobList.Items[0]
	exec.events <- &evPipelineStatus{
		pipeline: pipeline,
		instance: pipeline.Instances[0],
		jobID:    job.UID,
		status: batch_v1.JobStatus{
			Failed: 5,
		},
	}

	exec.runOnce(timeout)
	exec.runOnce(timeout)
	exec.runOnce(timeout)

	if pipeline.State != StateStopped {
		t.Error(pipeline.State)
	}
}

func TestPipelineRestart(t *testing.T) {
	exec := &mrExecutor{
		pipelines: make(map[string]*Pipeline),
		dataDir:   "testdata",
		events:    make(chan smEvent, 16),
		k8sClient: fake.NewSimpleClientset(),
	}

	config := &Config{
		Spec: &Spec{
			Name:      "test",
			Namespace: "roque",
			Storage:   "gs://laserlike_roque/test",
			Tasks: []TaskSpec{
				{
					Name: "step1",
					TemplateList: []*JobTemplate{
						&JobTemplate{
							Job:         "1a",
							Image:       "step1",
							Instances:   4,
							Parallelism: 4,
						},
						&JobTemplate{
							Job:         "1b",
							Image:       "step1",
							Instances:   4,
							Parallelism: 2,
						},
					},
				},
			},
		},
	}

	defaultPipelineSpecValues(config.Spec, "../../templates")
	pipeline := &Pipeline{
		Name:   "test",
		State:  StateStopped,
		Config: config,
	}
	exec.pipelines[pipeline.Name] = pipeline

	exec.SetState(pipeline, ActionStart, 0, 0)

	timeout := time.NewTicker(time.Second)
	exec.runOnce(timeout)

	if pipeline.State != StateRunning {
		t.Error(pipeline.State)
	}

	exec.runOnce(timeout)
	jobList, err := exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobList.Items) != 2 {
		t.Error(len(jobList.Items))
	}

	exec.SetState(pipeline, ActionStop, 1, 0)
	exec.runOnce(timeout)

	jobList, err = exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var completions int32
	for _, j := range jobList.Items {
		completions += *j.Spec.Completions
	}

	if completions != 0 {
		t.Fatal(completions)
	}

	exec.runOnce(timeout)
	if pipeline.State != StateStopped {
		t.Error(pipeline.State)
	}

	exec.SetState(pipeline, ActionStart, 1, 0)
	exec.runOnce(timeout)
	if pipeline.State != StateRunning {
		t.Error(pipeline.State)
	}

	exec.runOnce(timeout)
	jobList, err = exec.k8sClient.BatchV1().Jobs(config.Spec.Namespace).List(api_v1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobList.Items) != 2 {
		t.Error(len(jobList.Items))
	}

}
