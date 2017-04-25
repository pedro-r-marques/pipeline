package pipeline

import (
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
)

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

	exec.events <- &evPipelineStatus{
		pipeline: pipeline,
		instance: pipeline.Instances[0],
		status: batch_v1.JobStatus{
			Conditions: []batch_v1.JobCondition{
				{
					Type: batch_v1.JobComplete,
				},
			},
		},
	}

	exec.runOnce(timeout)
}
