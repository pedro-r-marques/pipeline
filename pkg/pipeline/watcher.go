package pipeline

import (
	"log"
	"strconv"
	"time"

	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/labels"
	"k8s.io/client-go/pkg/watch"
)

// Watcher monitors the kubernetes api-server for events concerning a specific pipeline
// instance.
type Watcher struct {
	pipeline  *Pipeline
	instance  *Instance
	selector  api_v1.ListOptions
	shutdown  chan struct{}
	terminate bool
}

func (w *Watcher) runOnce(podWatcher, jobWatcher watch.Interface, eventChan chan smEvent) bool {
	select {
	case ev, ok := <-podWatcher.ResultChan():
		if !ok {
			return false
		}
		switch ev.Type {
		case watch.Added:
		case watch.Deleted:
		case watch.Modified:
			pod := ev.Object.(*api_v1.Pod)
			log.Println("POD", ev.Type, pod.Name)
		}

	case ev, ok := <-jobWatcher.ResultChan():
		if !ok {
			return false
		}
		switch ev.Type {
		case watch.Added:
		case watch.Modified:
			job := ev.Object.(*batch_v1.Job)
			log.Println("Job", ev.Type, job.Name)
			eventChan <- &evPipelineStatus{
				w.pipeline,
				w.instance,
				job.UID,
				job.Status,
			}
		case watch.Deleted:
		}

	case <-w.shutdown:
		w.terminate = true
		return false
	}
	return true
}

func (w *Watcher) createWatchers(clientset kubernetes.Interface) (watch.Interface, watch.Interface, error) {
	podWatcher, err := clientset.Core().Pods(w.pipeline.Config.Spec.Namespace).Watch(w.selector)
	if err != nil {
		return nil, nil, err
	}
	jobWatcher, err := clientset.BatchV1().Jobs(w.pipeline.Config.Spec.Namespace).Watch(w.selector)
	if err != nil {
		return nil, nil, err
	}
	return podWatcher, jobWatcher, nil
}

// Run executes the listening loop in a goroutine.
func (w *Watcher) Run(clientset kubernetes.Interface, eventChan chan smEvent) {
	for !w.terminate {
		podWatcher, jobWatcher, err := w.createWatchers(clientset)
		if err != nil {
			log.Println(err)
			time.Sleep(5 * time.Second)
			continue
		}
		for w.runOnce(podWatcher, jobWatcher, eventChan) {
		}
	}
}

// Shutdown terminates the loop
func (w *Watcher) Shutdown() {
	w.shutdown <- struct{}{}
}

// MakeWatcher creates an object that monitors a specific pipeline instance
func MakeWatcher(p *Pipeline, instance *Instance) *Watcher {
	listOpt := api_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{
			"pipeline": p.Name,
			"id":       strconv.Itoa(instance.ID),
		})).String(),
	}

	return &Watcher{
		pipeline: p,
		instance: instance,
		selector: listOpt,
		shutdown: make(chan struct{}),
	}
}
