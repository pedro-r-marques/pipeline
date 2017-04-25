package pipeline

import (
	"bytes"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	api "k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/util/yaml"
)

// TemplateVars defines the variables passed to the yaml template
type TemplateVars struct {
	Pipeline    map[string]string // Pipeline parameters
	Task        map[string]string // Task parameters
	Instances   int
	Parallelism int
	Args        []string // Container arguments
	Resources   api.ResourceRequirements
}

// ServiceVars defines the variables passed to the service yaml template
type ServiceVars struct {
	Pipeline map[string]string
	Service  map[string]string
	Ports    []PortSpec
}

func isResourceSpecSet(rs *api.ResourceRequirements) bool {
	return isResourceListSet(rs.Requests) || isResourceListSet(rs.Limits)
}

func isResourceListSet(r api.ResourceList) bool {
	return len(r) > 0
}

func printResourceList(rlist api.ResourceList, indent int) string {
	var repr string
	var count int
	for k, v := range rlist {
		if count > 0 {
			repr += "\n"
			for i := 0; i < indent; i++ {
				repr += " "
			}
		}
		repr += string(k) + ": " + v.String()
		count++
	}
	return repr
}

var templateFuncs = template.FuncMap{
	"isResourceSpecSet": isResourceSpecSet,
	"isResourceListSet": isResourceListSet,
	"printResourceList": printResourceList,
}

// pathJoin is used because filepath.Join replaces consecutive slashes such as
// gs://path.
func pathJoin(dir, name string) string {
	if dir == "" {
		return name
	}
	if dir[len(dir)-1] == '/' {
		return dir + name
	}
	return dir + "/" + name
}

// func genK8SConfFilename(spec *Spec, id int, name string) string {
// 	return fmt.Sprintf("%s/%d/conf/%s.yaml", spec.Storage, id, name)
// }

func expandTemplateArgs(vars *TemplateVars, args []string) ([]string, error) {
	if args == nil || len(args) == 0 {
		return nil, nil
	}
	content := strings.Join(args, "\n")
	tmpl, err := template.New("").Parse(content)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	err = tmpl.Execute(&output, vars)
	if err != nil {
		return nil, err
	}
	return strings.Split(output.String(), "\n"), nil
}

func makeTemplateVars(spec *Spec, id int, task *TaskSpec, tmpl *JobTemplate) *TemplateVars {
	tmplVars := &TemplateVars{
		Pipeline: make(map[string]string),
		Task:     make(map[string]string),
	}
	tmplVars.Pipeline["Name"] = spec.Name
	tmplVars.Pipeline["TaskPrefix"] = strings.Replace(spec.Name, "_", "-", -1)
	tmplVars.Pipeline["Namespace"] = spec.Namespace
	tmplVars.Pipeline["ID"] = strconv.Itoa(id)

	if tmpl.Job != "" {
		tmplVars.Task["Name"] = tmpl.Job
	} else {
		tmplVars.Task["Name"] = task.Name
	}

	if spec.Storage != "" {
		tmplVars.Pipeline["WorkDir"] = pathJoin(spec.Storage, strconv.Itoa(id))
	}
	if task.EtcdLock != "" {
		tmplVars.Task["EtcdLock"] = task.EtcdLock
	}

	tmplVars.Instances = tmpl.Instances
	tmplVars.Parallelism = tmpl.Parallelism
	tmplVars.Task["Image"] = tmpl.Image
	tmplVars.Resources = tmpl.Resources
	if args, err := expandTemplateArgs(tmplVars, tmpl.Args); err == nil {
		tmplVars.Args = args
	} else {
		log.Println(err)
	}
	return tmplVars
}

func makeServiceVars(spec *Spec, id int, task *TaskSpec, svc *ServiceSpec) *ServiceVars {
	svcVars := &ServiceVars{
		Pipeline: make(map[string]string),
		Service:  make(map[string]string),
		Ports:    svc.Ports,
	}

	svcVars.Pipeline["Name"] = spec.Name
	svcVars.Pipeline["SvcPrefix"] = strings.Replace(spec.Name, "_", "-", -1)
	svcVars.Pipeline["Namespace"] = spec.Namespace
	svcVars.Pipeline["ID"] = strconv.Itoa(id)
	if spec.Storage != "" {
		svcVars.Pipeline["WorkDir"] = pathJoin(spec.Storage, strconv.Itoa(id))
	}

	svcVars.Service["Name"] = svc.Name
	if svc.Job == "" {
		svcVars.Service["Task"] = task.Name
	} else {
		svcVars.Service["Task"] = svc.Job
	}
	return svcVars
}

func createK8SConfig(tmplFile string, writer io.Writer, vars interface{}) error {
	rd, err := newFileReader(tmplFile)
	if err != nil {
		return err
	}
	defer rd.Close()

	content, err := ioutil.ReadAll(rd)
	if err != nil {
		return err
	}
	tmpl, err := template.New("").Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return err
	}

	return tmpl.Execute(writer, vars)
}

func makeK8SJobSpecFromSpec(spec *Spec, instanceID int, taskSpec *TaskSpec, jspec *JobTemplate) (*batch.Job, error) {
	buffer := new(bytes.Buffer)
	vars := makeTemplateVars(spec, instanceID, taskSpec, jspec)
	if err := createK8SConfig(jspec.Template, buffer, vars); err != nil {
		return nil, err
	}

	decoder := yaml.NewYAMLOrJSONDecoder(buffer, 4096)
	var job batch.Job
	if err := decoder.Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

// makeTaskFromSpec creates a Task from the pipeline spec and templates.
func makeTaskFromSpec(spec *Spec, instanceID int, taskSpec *TaskSpec) (*Task, error) {
	var jobs []*batch.Job

	for _, jspec := range taskSpec.JobSpecs() {
		job, err := makeK8SJobSpecFromSpec(spec, instanceID, taskSpec, jspec)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	task := &Task{
		jobs: jobs,
	}
	return task, nil
}
