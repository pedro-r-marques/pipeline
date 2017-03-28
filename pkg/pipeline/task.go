package pipeline

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	api "k8s.io/client-go/pkg/api/v1"
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

func resourceSpecSet(rs *api.ResourceRequirements) bool {
	return resourceSet(rs.Requests) || resourceSet(rs.Limits)
}

func resourceSet(r api.ResourceList) bool {
	return len(r) == 0
}

var templateFuncs = template.FuncMap{
	"resourceSpecSet": resourceSpecSet,
	"resourceSet":     resourceSet,
}

func genK8SConfFilename(spec *Spec, id int, name string) string {
	return fmt.Sprintf("%s/%d/conf/%s.yaml", spec.Storage, id, name)
}

func pathJoin(dir, name string) string {
	if dir == "" {
		return name
	}
	if dir[len(dir)-1] == '/' {
		return dir + name
	}
	return dir + "/" + name
}

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
	tmplVars.Pipeline["WorkDir"] = pathJoin(spec.Storage, strconv.Itoa(id))
	tmplVars.Task["EtcdLock"] = task.EtcdLock
	tmplVars.Instances = tmpl.Instances
	tmplVars.Parallelism = tmpl.Parallelism
	if tmpl.Job != "" {
		tmplVars.Task["Name"] = tmpl.Job
	} else {
		tmplVars.Task["Name"] = task.Name
	}
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
	svcVars.Pipeline["WorkDir"] = pathJoin(spec.Storage, strconv.Itoa(id))

	svcVars.Service["Name"] = svc.Name
	if svc.Job == "" {
		svcVars.Service["Task"] = task.Name
	} else {
		svcVars.Service["Task"] = svc.Job
	}
	return svcVars
}

func createK8SConfig(tmplFile, outputFile string, vars interface{}) error {
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

	wr, err := newFileWriter(outputFile)
	if err != nil {
		return err
	}
	defer wr.Close()

	return tmpl.Execute(wr, vars)
}

func applyK8SConfig(uri string) error {
	var localfile string
	var rmFile bool
	if strings.HasPrefix(uri, fileScheme) {
		localfile = uri[len(fileScheme):]
	} else {
		var err error
		localfile, err = newLocalCopy(uri, "k8s")
		if err != nil {
			return err
		}
		rmFile = true
	}

	cmd := exec.Command("kubectl", "apply", "-f", localfile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(output))
	} else {
		if rmFile {
			os.Remove(localfile)
		}
		// log.V(3).Info(string(output))
	}
	return err
}

func (p *Pipeline) createTask(instance *Instance, stage int) error {
	if stage >= len(p.Spec.Tasks) {
		return fmt.Errorf("Invalid stage %d", stage)
	}

	taskSpec := &p.Spec.Tasks[stage]

	// generate k8s job configurations
	var configFiles []string

	if len(taskSpec.TemplateList) > 0 {
		for i := range taskSpec.TemplateList {
			tmpl := &taskSpec.TemplateList[i]
			filename := genK8SConfFilename(p.Spec, instance.ID, tmpl.Job)
			vars := makeTemplateVars(p.Spec, instance.ID, taskSpec, tmpl)
			if err := createK8SConfig(tmpl.Template, filename, vars); err == nil {
				configFiles = append(configFiles, filename)
				// glog.V(1).Infof("Created %s for %s-%d task: %s", filename, p.Name, instance.ID, taskSpec.Name)
			} else {
				log.Println(err)
			}
		}
	} else {
		filename := genK8SConfFilename(p.Spec, instance.ID, taskSpec.Name)
		vars := makeTemplateVars(p.Spec, instance.ID, taskSpec, &taskSpec.Template)
		if err := createK8SConfig(taskSpec.Template.Template, filename, vars); err != nil {
			log.Println(err)
			return err
		}
		configFiles = []string{filename}
		// glog.V(1).Infof("Created %s for %s-%d task: %s", filename, p.Name, instance.ID, taskSpec.Name)
	}

	var err error
	for _, file := range configFiles {
		err = applyK8SConfig(file)
		if err != nil {
			log.Println(err)
			break
		}
	}
	return err
}

func (p *Pipeline) deleteInstanceResourceAll(instance *Instance, resource string) {
	args := []string{
		"--namespace=" + p.Spec.Namespace,
		"delete", resource,
		"-l", "pipeline=" + p.Spec.Name + ",id=" + strconv.Itoa(instance.ID),
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(output))
	} else {
		// glog.V(3).Info(string(output))
	}
}

func deleteTaskResource(baseArgs []string, name string) error {
	taskArgs := append(
		baseArgs[:len(baseArgs)-1], baseArgs[len(baseArgs)-1]+",task="+name)
	cmd := exec.Command("kubectl", taskArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(output))
	} else {
		// glog.V(3).Info(string(output))
	}
	return err
}

func (p *Pipeline) deleteInstanceResource(instance *Instance, resource string, taskIndex int) {
	if taskIndex == 0 {
		p.deleteInstanceResourceAll(instance, resource)
		return
	}

	args := []string{
		"--namespace=" + p.Spec.Namespace,
		"delete", resource,
		"-l", "pipeline=" + p.Spec.Name + ",id=" + strconv.Itoa(instance.ID),
	}

	for i := taskIndex; i < len(p.Spec.Tasks); i++ {
		task := &p.Spec.Tasks[i]
		if len(task.TemplateList) > 0 {
			for k := range task.TemplateList {
				tmpl := &task.TemplateList[k]
				deleteTaskResource(args, tmpl.Job)
			}
		} else {
			deleteTaskResource(args, task.Name)
		}
	}
}

func (p *Pipeline) createService(instance *Instance, task *TaskSpec, svc *ServiceSpec) error {
	vars := makeServiceVars(p.Spec, instance.ID, task, svc)
	filename := genK8SConfFilename(p.Spec, instance.ID, "service-"+svc.Name)
	if err := createK8SConfig(svc.Template, filename, vars); err != nil {
		return err
	}
	return applyK8SConfig(filename)
}

func (p *Pipeline) createServices(instance *Instance, stage int) error {
	if stage >= len(p.Spec.Tasks) {
		return fmt.Errorf("Invalid stage %d", stage)
	}

	taskSpec := &p.Spec.Tasks[stage]

	for i := 0; i < len(taskSpec.Services); i++ {
		if err := p.createService(instance, taskSpec, &taskSpec.Services[i]); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) stopInstance(instance *Instance) {
	args := []string{
		"--namespace=" + p.Spec.Namespace,
		"resize", "--replicas=0",
	}

	for _, status := range instance.JobsStatus {
		if !status.IsRunning() {
			continue
		}
		cmdArgs := append(args, "job/"+status.JobName)
		// glog.V(3).Infof("resize job %s", status.JobName)
		cmd := exec.Command("kubectl", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Println(string(output))
		} else {
			// glog.V(3).Info(string(output))
		}
	}
}
