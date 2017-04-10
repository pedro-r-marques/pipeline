package pipeline

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/labels"
)

// Task contains the kubernetes definition for a task.
type Task struct {
	svc  *api.Service
	jobs []*batch.Job
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

	// taskSpec := &p.Spec.Tasks[stage]

	// // generate k8s job configurations
	// var configFiles []string

	// if len(taskSpec.TemplateList) > 0 {
	// 	for i := range taskSpec.TemplateList {
	// 		tmpl := taskSpec.TemplateList[i]
	// 		filename := genK8SConfFilename(p.Spec, instance.ID, tmpl.Job)
	// 		vars := makeTemplateVars(p.Spec, instance.ID, taskSpec, tmpl)
	// 		if err := createK8SConfig(tmpl.Template, filename, vars); err == nil {
	// 			configFiles = append(configFiles, filename)
	// 			// glog.V(1).Infof("Created %s for %s-%d task: %s", filename, p.Name, instance.ID, taskSpec.Name)
	// 		} else {
	// 			log.Println(err)
	// 		}
	// 	}
	// } else {
	// 	filename := genK8SConfFilename(p.Spec, instance.ID, taskSpec.Name)
	// 	vars := makeTemplateVars(p.Spec, instance.ID, taskSpec, &taskSpec.JobTemplate)
	// 	if err := createK8SConfig(taskSpec.Template, filename, vars); err != nil {
	// 		log.Println(err)
	// 		return err
	// 	}
	// 	configFiles = []string{filename}
	// 	// glog.V(1).Infof("Created %s for %s-%d task: %s", filename, p.Name, instance.ID, taskSpec.Name)
	// }

	var err error
	// for _, file := range configFiles {
	// 	err = applyK8SConfig(file)
	// 	if err != nil {
	// 		log.Println(err)
	// 		break
	// 	}
	// }
	return err
}

func deleteJobsAndServicesForSelector(k8sClient *kubernetes.Clientset, namespace string, listOpt *api_v1.ListOptions) {
	jobsClient := k8sClient.BatchV1().Jobs(namespace)
	if jobList, err := jobsClient.List(*listOpt); err == nil {
		for _, job := range jobList.Items {
			jobsClient.Delete(job.Name, nil)
		}
	} else {
		log.Println(err)
	}

	if svcList, err := k8sClient.Services(namespace).List(*listOpt); err == nil {
		for _, svc := range svcList.Items {
			k8sClient.Services(namespace).Delete(svc.Name, nil)
		}
	} else {
		log.Println(err)
	}
}
func (p *Pipeline) deleteInstanceResources(k8sClient *kubernetes.Clientset, instance *Instance) {
	listOpt := api_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"pipeline": p.Spec.Name,
			"id":       strconv.Itoa(instance.ID),
		})).String(),
	}
	deleteJobsAndServicesForSelector(k8sClient, p.Spec.Namespace, &listOpt)
}

func (p *Pipeline) deleteTaskResources(k8sClient *kubernetes.Clientset, instance *Instance, taskIndex int) {
	if taskIndex == 0 {
		p.deleteInstanceResources(k8sClient, instance)
		return
	}

	listOpt := api_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"pipeline": p.Spec.Name,
			"id":       strconv.Itoa(instance.ID),
			"task":     "?",
		})).String(),
	}
	deleteJobsAndServicesForSelector(k8sClient, p.Spec.Namespace, &listOpt)

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
	// args := []string{
	// 	"--namespace=" + p.Spec.Namespace,
	// 	"resize", "--replicas=0",
	// }

	// for _, status := range instance.JobsStatus {
	// 	if !status.IsRunning() {
	// 		continue
	// 	}
	// 	cmdArgs := append(args, "job/"+status.JobName)
	// 	// glog.V(3).Infof("resize job %s", status.JobName)
	// 	cmd := exec.Command("kubectl", cmdArgs...)
	// 	output, err := cmd.CombinedOutput()
	// 	if err != nil {
	// 		log.Println(string(output))
	// 	} else {
	// 		// glog.V(3).Info(string(output))
	// 	}
	// }
}
