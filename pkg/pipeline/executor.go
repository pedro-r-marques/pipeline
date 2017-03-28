package pipeline

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Executor is the interface for the executor class.
type Executor interface {
	PipelineAdd(name, uri string) error
	SetState(p *Pipeline, action StateAction, instanceID int, stage int) error
	Clone(p *Pipeline, id int, includePat, excludePat string) error
	PipelineMapKeys(pattern *regexp.Regexp) []string
	PipelineCount() int
	PipelineLookup(name string) *Pipeline
	PipelineReload(p *Pipeline) error
	PipelineDelete(p *Pipeline)
	DeleteInstance(p *Pipeline, instanceID int)

	Start()
	Configure(uri string) error
	SetCheckpointFile(uri string)
}

type mrExecutor struct {
	sync.Mutex
	pipelines      map[string]*Pipeline
	checkpointFile string
	dataDir        string
	events         chan smEvent
	cron           Cron
}

func (exec *mrExecutor) PipelineLookup(name string) *Pipeline {
	exec.Lock()
	defer exec.Unlock()
	return exec.pipelines[name]
}

// PipelineAdd executes from an http server goroutine.
func (exec *mrExecutor) PipelineAdd(name, uri string) error {
	// fetch the configuration from the storage service
	rd, err := newFileReader(uri)
	if err != nil {
		return err
	}
	defer rd.Close()

	// parse the configuration file
	spec, err := parsePipelineConfig(rd, exec.dataDir)
	if err != nil {
		return err
	}

	p := &Pipeline{
		Name:  name,
		URI:   uri,
		State: StateStopped,
		Spec:  spec,
	}
	exec.Lock()
	defer exec.Unlock()

	exec.pipelines[name] = p
	if p.Spec.Schedule != nil {
		t := &pipelineTrigger{exec, p}
		exec.cron.Add(p.Name, p.Spec.Schedule, t.trigger)
	}
	return nil
}

func (exec *mrExecutor) PipelineReload(p *Pipeline) error {
	rd, err := newFileReader(p.URI)
	if err != nil {
		return err
	}
	defer rd.Close()

	// parse the configuration file
	spec, err := parsePipelineConfig(rd, exec.dataDir)
	if err != nil {
		return err
	}

	if p.Spec.Schedule != nil {
		exec.cron.Delete(p.Name)
	}

	p.Spec = spec
	for _, instance := range p.Instances {
		instance.JobsStatus = nil
		instance.Stage = 0
	}

	if p.Spec.Schedule != nil {
		t := &pipelineTrigger{exec, p}
		exec.cron.Add(p.Name, p.Spec.Schedule, t.trigger)
	}

	return nil
}

func (exec *mrExecutor) PipelineDelete(p *Pipeline) {
	exec.Lock()
	defer exec.Unlock()
	delete(exec.pipelines, p.Name)
	if p.Spec.Schedule != nil {
		exec.cron.Delete(p.Name)
	}
}

func (exec *mrExecutor) DeleteInstance(p *Pipeline, instanceID int) {
	exec.events <- &evInstanceDelete{p, instanceID}
}

func (exec *mrExecutor) PipelineMapKeys(pattern *regexp.Regexp) []string {
	exec.Lock()
	defer exec.Unlock()
	var keys []string
	for k := range exec.pipelines {
		if pattern == nil || pattern.MatchString(k) {
			keys = append(keys, k)
		}
	}
	return keys
}

func (exec *mrExecutor) PipelineCount() int {
	return len(exec.pipelines)
}

type pipelineTrigger struct {
	exec *mrExecutor
	p    *Pipeline
}

func (t *pipelineTrigger) trigger() {
	// glog.V(2).Info("trigger for ", t.p.Name)
	instance := t.p.createInstance()
	t.exec.events <- &evPipelineRun{t.p, instance.ID, 0}
}

func (exec *mrExecutor) SetState(p *Pipeline, action StateAction, instanceID int, stage int) error {
	switch action {
	case ActionStart:
		if instanceID == 0 {
			// start new instance
			instance := p.createInstance()
			exec.events <- &evPipelineRun{p, instance.ID, 0}
		} else {
			// restart an existing instance
			instance := p.getInstance(instanceID)
			if instance == nil {
				return fmt.Errorf("Instance id %d not found", instanceID)
			}
			exec.events <- &evPipelineRun{p, instanceID, stage}
		}
	case ActionStop:
		if instanceID == 0 {
			// stop all instances
		} else {
			// stop a specific instance
			instance := p.getInstance(instanceID)
			if instance == nil {
				return fmt.Errorf("Invalid instance ID %d", instanceID)
			}
			exec.events <- &evTaskAbort{p, instanceID, instance.Stage, "User request", time.Now()}
		}
	}
	return nil
}

func (exec *mrExecutor) Clone(p *Pipeline, prevID int, includePat, excludePat string) error {
	var reIncl, reExcl *regexp.Regexp
	if includePat != "" {
		var err error
		if reIncl, err = regexp.Compile(includePat); err != nil {
			return err
		}
	}
	if excludePat != "" {
		var err error
		if reExcl, err = regexp.Compile(excludePat); err != nil {
			return err
		}
	}

	instance := p.createInstance()
	prevDir := p.Spec.Storage + "/" + strconv.Itoa(prevID)
	workDir := p.Spec.Storage + "/" + strconv.Itoa(instance.ID)
	return copyWorkDir(prevDir, workDir, reIncl, reExcl)
}

func (exec *mrExecutor) Start() {
	go exec.run()
}

func (exec *mrExecutor) Configure(uri string) error {
	rd, err := newFileReader(uri)
	if err != nil {
		return err
	}
	defer rd.Close()
	var pipelines map[string]*Pipeline
	decoder := json.NewDecoder(rd)
	if err := decoder.Decode(&pipelines); err != nil {
		return err
	}
	exec.pipelines = pipelines
	for _, p := range pipelines {
		if p.Spec.Schedule == nil {
			continue
		}
		t := &pipelineTrigger{exec, p}
		exec.cron.Add(p.Name, p.Spec.Schedule, t.trigger)
	}
	return nil
}

func (exec *mrExecutor) SetCheckpointFile(uri string) {
	exec.checkpointFile = uri
}

func (exec *mrExecutor) checkpointConfig() {
	wr, err := newFileWriter(exec.checkpointFile)
	if err != nil {
		log.Println(err)
		return
	}
	defer wr.Close()

	exec.Lock()
	js, err := json.Marshal(exec.pipelines)
	exec.Unlock()
	if err != nil {
		log.Println(err)
	}

	wr.Write(js)
}

// NewExecutor allocates an Executor.
func NewExecutor(dataDir string) Executor {
	return &mrExecutor{
		pipelines: make(map[string]*Pipeline),
		dataDir:   dataDir,
		events:    make(chan smEvent, 16),
		cron:      NewCronExecutor(),
	}
}
