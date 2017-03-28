package pipeline

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/go-yaml/yaml"

	api "k8s.io/client-go/pkg/api/v1"
)

// Spec defines the specification for a pipeline.
type Spec struct {
	// Name defines the pipeline name
	Name string
	// Namespace specifies the kubernetes namespace for the data pipeline.
	Namespace string
	// Google storage directory (e.g. gs://laserlike_roque/mr_sitedata
	Storage string

	// Schedule defines a crontab style schedule.
	Schedule *CronSchedule `json:",omitempty"`

	Tasks []TaskSpec
}

// TaskSpec defines the tasks to execute for this pipeline.
type TaskSpec struct {
	// Task name
	Name string

	EtcdLock string `yaml:"etcd_lock"`

	Services     []ServiceSpec `yaml:"services"`
	Template     JobTemplate   `yaml:",inline"`
	TemplateList []JobTemplate `yaml:"jobs"`
}

// ServiceSpec defines the specification for a service.
type ServiceSpec struct {
	// Service is the name of the service
	Name string
	// Template: defaults to default-service-template.yaml
	Template string
	Job      string
	Ports    []PortSpec
}

// PortSpec specifies a service port.
type PortSpec struct {
	Name string
	Port int
}

// JobTemplate defines the parameters for a k8s job.
type JobTemplate struct {
	// Job defines the k8s name of the job.
	Job string
	// Template: defaults to default-job-template.yaml
	Template string
	// Image to execute (mandatory)
	Image string
	// Instances (defaults to 1)
	Instances int

	// Container arguments
	Args []string

	// Parallelism (defaults to number of instances)
	Parallelism int

	Resources api.ResourceRequirements
}

func defaultJobTemplateValues(tmpl *JobTemplate, dataDir string) {
	if tmpl.Template == "" {
		tmpl.Template = "file://" + dataDir + "/default-job-template.yaml"
	}
	if tmpl.Instances == 0 {
		tmpl.Instances = 1
	}
	if tmpl.Parallelism == 0 {
		tmpl.Parallelism = tmpl.Instances
	}
}

func defaultServiceValues(task *TaskSpec, dataDir string) {
	for i := 0; i < len(task.Services); i++ {
		svc := &task.Services[i]
		if svc.Template == "" {
			svc.Template = "file://" + dataDir + "/default-service-template.yaml"
		}
	}
}

func defaultPipelineSpecValues(spec *Spec, dataDir string) {
	for i := range spec.Tasks {
		task := &spec.Tasks[i]
		if len(task.TemplateList) == 0 {
			defaultJobTemplateValues(&task.Template, dataDir)
		} else {
			for k := range task.TemplateList {
				tmpl := &task.TemplateList[k]
				defaultJobTemplateValues(tmpl, dataDir)
			}
		}
		defaultServiceValues(task, dataDir)
	}
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}

func isJobTemplateEmpty(tmpl *JobTemplate) bool {
	return tmpl.Image == "" && tmpl.Template == "" && tmpl.Instances == 0
}

func validateJobTemplate(tmpl *JobTemplate) error {
	if tmpl.Image == "" {
		return &validationError{"image must be specifed for task"}
	}
	if tmpl.Parallelism > tmpl.Instances {
		return &validationError{"parallelism must be less or equal than number of instances"}
	}
	return nil
}

func getTaskJobByName(task *TaskSpec, name string) *JobTemplate {
	for i := 0; i < len(task.TemplateList); i++ {
		tmpl := &task.TemplateList[i]
		if tmpl.Job == name {
			return tmpl
		}
	}
	return nil
}

func validateTaskService(task *TaskSpec, svc *ServiceSpec) error {
	if svc.Name == "" {
		return &validationError{"Service name not defined"}
	}
	if svc.Job != "" && getTaskJobByName(task, svc.Job) == nil {
		return &validationError{fmt.Sprintf("unknown job %s in service %s", svc.Job, svc.Name)}
	}
	for i := 0; i < len(svc.Ports); i++ {
		portSpec := &svc.Ports[i]
		if portSpec.Name == "" {
			return &validationError{"Port name must be defined"}
		}
		if portSpec.Port == 0 {
			return &validationError{"Invalid port"}
		}
	}
	return nil
}

func validatePipelineConfig(spec *Spec) error {
	if spec.Name == "" {
		return &validationError{"pipeline name must be specified"}
	}
	if spec.Storage != "" && !strings.HasPrefix(spec.Storage, "gs://") {
		return &validationError{"unsupported storage method"}
	}

	for i := range spec.Tasks {
		task := &spec.Tasks[i]
		if len(task.TemplateList) == 0 {
			if err := validateJobTemplate(&task.Template); err != nil {
				return err
			}

		} else {
			if !isJobTemplateEmpty(&task.Template) {
				return &validationError{"task template and template-list are mutually exclusive"}
			}
			for k := range task.TemplateList {
				tmpl := &task.TemplateList[k]
				if err := validateJobTemplate(tmpl); err != nil {
					return err
				}
			}
		}
		for k := range task.Services {
			if err := validateTaskService(task, &task.Services[k]); err != nil {
				return err
			}
		}
	}

	return nil
}

func cleanURI(uri string) string {
	if i := strings.Index(uri, "://"); i >= 0 {
		path := uri[i+3:]
		return uri[0:i+3] + filepath.Clean(path)
	}
	return uri
}

func canonicalizeSpecValues(s *Spec) {
	s.Storage = cleanURI(s.Storage)
}

func parsePipelineConfig(rd io.Reader, dataDir string) (*Spec, error) {
	content, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	var spec Spec
	err = yaml.Unmarshal(content, &spec)
	if err != nil {
		return nil, err
	}
	defaultPipelineSpecValues(&spec, dataDir)
	canonicalizeSpecValues(&spec)
	err = validatePipelineConfig(&spec)
	return &spec, err
}
