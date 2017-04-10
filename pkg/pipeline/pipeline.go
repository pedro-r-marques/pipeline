package pipeline

// ExecState defines the state of a job
type ExecState string

const (
	// StateStopped means that the job is configured but not running
	StateStopped ExecState = "Stopped"
	// StateRunning means that the job is currently executing
	StateRunning ExecState = "Running"
)

// Pipeline defines a data processing pipeline.
// Pipelines define a sequence of tasks to be executed using storage as a data
// transfer mechanism between tasks.
// A Pipeline is executed multiple time times with potentially different datasets.
type Pipeline struct {
	Name  string    `json:"name"`
	URI   string    `json:"uri"`
	Spec  *Spec     `json:"spec"`
	State ExecState `json:"state"`

	Instances []*Instance
}

func (p *Pipeline) createInstance() *Instance {
	var max int
	for _, instance := range p.Instances {
		if instance.ID > max {
			max = instance.ID
		}
	}
	instance := makeInstance(max + 1)
	p.Instances = append(p.Instances, instance)
	return instance
}

func (p *Pipeline) getInstance(id int) *Instance {
	for _, instance := range p.Instances {
		if instance.ID == id {
			return instance
		}
	}
	return nil
}

func (p *Pipeline) deleteInstance(target *Instance) {
	p.deleteInstanceResources(nil, target)

	for i, instance := range p.Instances {
		if instance == target {
			p.Instances = append(p.Instances[:i], p.Instances[i+1:]...)
			break
		}
	}
}

func (p *Pipeline) numStages() int {
	var count int
	for i := range p.Spec.Tasks {
		t := &p.Spec.Tasks[i]
		if len(t.TemplateList) > 0 {
			count += len(t.TemplateList)
		} else {
			count++
		}
	}
	return count
}

func (p *Pipeline) getStageIndex(taskName string) (int, int) {
	for i := range p.Spec.Tasks {
		t := &p.Spec.Tasks[i]
		if len(t.TemplateList) > 0 {
			for k := 0; k < len(t.TemplateList); k++ {
				if t.TemplateList[k].Job == taskName {
					return i, k
				}
			}
		} else {
			if t.Name == taskName {
				return i, 0
			}
		}
	}
	return -1, -1
}
