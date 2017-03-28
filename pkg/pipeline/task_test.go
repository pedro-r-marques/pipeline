package pipeline

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"text/template"
)

func TestTemplateVars(t *testing.T) {
	testCases := []struct {
		template string
		spec     *Spec
		expected string
	}{
		{
			"name: {{.Task.Name}}-{{.Pipeline.ID}}",
			&Spec{
				Tasks: []TaskSpec{
					TaskSpec{Name: "pipeline-job"},
				},
			},
			"name: pipeline-job-1",
		},
		{
			"namespace: {{.Pipeline.Namespace}}",
			&Spec{
				Namespace: "roque",
				Tasks: []TaskSpec{
					TaskSpec{},
				},
			},
			"namespace: roque",
		},
	}

	for i := range testCases {
		test := &testCases[i]
		tmpl, err := template.New("").Parse(test.template)
		if err != nil {
			t.Error(err)
			continue
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, makeTemplateVars(
			test.spec, 1, &test.spec.Tasks[0], &test.spec.Tasks[0].Template))
		if err != nil {
			t.Error(err)
			continue
		}
		result := string(buf.Bytes())
		if result != test.expected {
			t.Errorf("expected %s, got %s", test.expected, result)
		}
	}
}

func TestDefaultTemplate(t *testing.T) {
	testCases := []struct {
		specFile   string
		stage      int
		subTask    int
		outputFile string
	}{
		{"testdata/basic.yaml", 1, 0, "testdata/k8s_token_allocator.yaml"},
		{"testdata/basic.yaml", 2, 2, "testdata/k8s_cofilter_compute.yaml"},
		{"testdata/etcdLock.yaml", 1, 0, "testdata/k8s_normalize.yaml"},
	}

	for i := range testCases {
		test := &testCases[i]
		fp, err := os.Open(test.specFile)
		if err != nil {
			t.Error(err)
		}
		defer fp.Close()
		spec, err := parsePipelineConfig(fp, "data")
		if err != nil {
			t.Error(test.specFile, err)
			continue
		}
		if len(spec.Tasks) < test.stage {
			t.Errorf("pipeline %s has %d tasks", spec.Name, len(spec.Tasks))
			continue
		}
		taskSpec := &spec.Tasks[test.stage-1]
		var job *JobTemplate
		if len(taskSpec.TemplateList) > 0 {
			if test.subTask < len(taskSpec.TemplateList) {
				t.Errorf("invalid subTask")
				continue
			}
			job = &taskSpec.TemplateList[test.subTask-1]
		} else {
			job = &taskSpec.Template
		}

		tmpFile, err := ioutil.TempFile("", "TestDefaultTemplate")
		defer tmpFile.Close()
		if err != nil {
			t.Error(err)
			continue
		}

		vars := makeTemplateVars(spec, 1, taskSpec, job)
		if err := createK8SConfig(job.Template, "file://"+tmpFile.Name(), vars); err != nil {
			t.Error(err)
		}

		cmd := exec.Command("diff", "-u", tmpFile.Name(), test.outputFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Error(string(output))
		}
	}
}

func TestServiceTemplate(t *testing.T) {
	testCases := []struct {
		specFile   string
		id         int
		taskIndex  int
		outputFile string
	}{
		{"testdata/service.yaml", 1, 0, "testdata/k8s_master_svc.yaml"},
	}
	for i := range testCases {
		test := &testCases[i]

		fp, err := os.Open(test.specFile)
		if err != nil {
			t.Error(err)
		}
		defer fp.Close()
		spec, err := parsePipelineConfig(fp, "data")
		if err != nil {
			t.Error(test.specFile, err)
			continue
		}

		if test.taskIndex >= len(spec.Tasks) {
			t.Fatal("Invalid taskIndex")
		}
		task := &spec.Tasks[test.taskIndex]
		if len(task.Services) == 0 {
			t.Fatal("No services defined")
		}
		svc := &task.Services[0]
		vars := makeServiceVars(spec, test.id, task, svc)
		tmpFile, err := ioutil.TempFile("", "TestServiceTemplate")
		defer tmpFile.Close()
		if err != nil {
			t.Error(err)
			continue
		}
		if err := createK8SConfig(svc.Template, "file://"+tmpFile.Name(), vars); err != nil {
			t.Error(err)
		}

		cmd := exec.Command("diff", "-u", tmpFile.Name(), test.outputFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Error(string(output))
		}
	}
}
