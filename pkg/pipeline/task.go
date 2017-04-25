package pipeline

import (
	"fmt"
	"log"
	"strconv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/labels"
	"k8s.io/client-go/pkg/types"
)

// Task contains the kubernetes definition for a task.
type Task struct {
	svc  *api.Service
	jobs []*batch_v1.Job

	// scheduled job IDs
	JobIDs map[string]types.UID

	completed int
}

func (t *Task) getJobByName(name string) *batch_v1.Job {
	for _, job := range t.jobs {
		if job.Name == name {
			return job
		}
	}
	return nil
}

func createTaskList(config *Config, instanceID int) ([]*Task, error) {
	var taskList []*Task
	spec := config.Spec
	for _, taskSpec := range spec.Tasks {
		task, err := makeTaskFromSpec(spec, instanceID, &taskSpec)
		if err != nil {
			return nil, err
		}
		taskList = append(taskList, task)
	}
	return taskList, nil
}

func (p *Pipeline) createTask(k8sClient kubernetes.Interface, instance *Instance, stage int) error {
	task := instance.TaskList[stage]
	for _, job := range task.jobs {
		j, err := k8sClient.BatchV1().Jobs(p.Config.Spec.Namespace).Create(job)
		if err != nil {
			return err
		}
		task.JobIDs[job.Name] = j.UID
	}
	return nil
}

func deleteJobsAndServicesForSelector(k8sClient kubernetes.Interface, namespace string, listOpt *api_v1.ListOptions) {
	jobsClient := k8sClient.BatchV1().Jobs(namespace)
	if jobList, err := jobsClient.List(*listOpt); err == nil {
		for _, job := range jobList.Items {
			jobsClient.Delete(job.Name, nil)
		}
	} else {
		log.Println(err)
	}

	if svcList, err := k8sClient.Core().Services(namespace).List(*listOpt); err == nil {
		for _, svc := range svcList.Items {
			k8sClient.Core().Services(namespace).Delete(svc.Name, nil)
		}
	} else {
		log.Println(err)
	}
}

func (p *Pipeline) deleteInstanceResources(k8sClient kubernetes.Interface, instance *Instance) {
	listOpt := api_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"pipeline": p.Name,
			"id":       strconv.Itoa(instance.ID),
		})).String(),
	}
	deleteJobsAndServicesForSelector(k8sClient, p.Config.Spec.Namespace, &listOpt)
}

func (p *Pipeline) deleteTaskResources(k8sClient kubernetes.Interface, instance *Instance, taskIndex int) {
	if taskIndex == 0 {
		p.deleteInstanceResources(k8sClient, instance)
		return
	}

	listOpt := api_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"pipeline": p.Name,
			"id":       strconv.Itoa(instance.ID),
			"task":     "?",
		})).String(),
	}
	deleteJobsAndServicesForSelector(k8sClient, p.Config.Spec.Namespace, &listOpt)

}

func (p *Pipeline) createService(instance *Instance, task *TaskSpec, svc *ServiceSpec) error {
	// vars := makeServiceVars(p.Spec, instance.ID, task, svc)
	// filename := genK8SConfFilename(p.Spec, instance.ID, "service-"+svc.Name)
	// if err := createK8SConfig(svc.Template, filename, vars); err != nil {
	// 	return err
	// }
	// return applyK8SConfig(filename)
	return nil
}

func (p *Pipeline) createServices(instance *Instance, stage int) error {
	if stage >= len(p.Config.Spec.Tasks) {
		return fmt.Errorf("Invalid stage %d", stage)
	}

	taskSpec := &p.Config.Spec.Tasks[stage]

	for i := 0; i < len(taskSpec.Services); i++ {
		if err := p.createService(instance, taskSpec, &taskSpec.Services[i]); err != nil {
			return err
		}
	}
	return nil
}

// cancelInstance changes the number of desired job replicas to 0.
func (p *Pipeline) cancelInstance(k8sClient kubernetes.Interface, instance *Instance) {
	task := instance.TaskList[instance.Stage]

	jobService := k8sClient.BatchV1().Jobs(p.Config.Spec.Namespace)
	for _, jcfg := range task.jobs {
		j, err := jobService.Get(jcfg.Name)
		if err != nil {
			log.Println(err)
			continue
		}
		if j.Status.Active > 0 {
			*j.Spec.Completions = 0
			_, err := jobService.Update(j)
			if err != nil {
				log.Println(err)
			}
		}
	}
}
