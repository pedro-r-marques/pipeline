package pipeline

import (
	"os"
	"reflect"
	"testing"
)

func TestParser(t *testing.T) {
	testCases := []struct {
		filename string
		spec     Spec
	}{
		{
			"testdata/basic.yaml",
			Spec{
				Name:      "mr_sitedata",
				Namespace: "roque",
				Storage:   "gs://laserlike_roque/mr",
				Tasks: []TaskSpec{
					{
						Name: "token-allocator",
						Template: JobTemplate{
							Template:    "file:///default-job-template.yaml",
							Image:       "gcr.io/laserlike-1167/roque-mr_cooccur_token_allocator",
							Instances:   1,
							Parallelism: 1,
							Args:        []string{"-v=3"},
						},
					},
					{
						Name: "compute",
						TemplateList: []JobTemplate{
							{
								Job:         "master",
								Template:    "file:///default-job-template.yaml",
								Image:       "gcr.io/laserlike-1167/roque-mr_cofilter_compute",
								Instances:   1,
								Parallelism: 1,
							},
							{
								Job:         "worker",
								Template:    "file:///default-job-template.yaml",
								Image:       "gcr.io/laserlike-1167/roque-mr_cofilter_compute",
								Instances:   16,
								Parallelism: 16,
								Resources: JobResourceSpec{
									Requests: Resources{"12Gi", "1"},
									Limits:   Resources{"14Gi", "1"},
								},
							},
						},
					},
				},
			},
		},
		{
			"testdata/cron.yaml",
			Spec{
				Name:      "periodic",
				Namespace: "roque",
				Storage:   "gs://laserlike_roque/pipeline",
				Schedule:  &CronSchedule{Min: "45", Hour: "1", Day: "15,30"},
				Tasks: []TaskSpec{
					{
						Name: "hello-world",
						Template: JobTemplate{
							Template:    "file:///default-job-template.yaml",
							Image:       "gcr.io/laserlike-1167/roque-hello-world",
							Instances:   1,
							Parallelism: 1,
						},
					},
				},
			},
		},
	}
	for i := range testCases {
		test := &testCases[i]
		fp, err := os.Open(test.filename)
		if err != nil {
			t.Error(err)
		}
		spec, err := parsePipelineConfig(fp, "")
		if err != nil {
			t.Error(test.filename, err)
			continue
		}
		if !reflect.DeepEqual(spec, &test.spec) {
			t.Errorf("expected %v, got %v", test.spec, spec)
		}
	}
}
