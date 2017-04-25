package pipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// APIServerURLPath is the URL path used for the REST API.
	APIServerURLPath = "/pipeline/api/"
)

// PipelinesPostRequest specifies the parameters for a POST request on the /pipelines service.
type PipelinesPostRequest struct {
	Name string
	URI  string
}

// DeleteRequest specifies the parameter for DELETE request on the /pipeline/<name> endpoint.
type DeleteRequest struct {
	InstanceID int `json:"instance"`
}

// StateAction defines the actions possible in the state API request
type StateAction string

const (
	// ActionStart directs the pipeline controller to (re)start a instance/task.
	ActionStart StateAction = "start"
	// ActionStop directs the pipeline controller to stop executing the specified pipeline.
	ActionStop StateAction = "stop"
)

// StateRequest is the API used to change the execution status of a pipeline.
type StateRequest struct {
	Action StateAction
	ID     int
	Stage  int
}

// APIServer implements http.HandlerFunc
type APIServer struct {
	exec Executor
}

// NewAPIServer allocates an APIServer object
func NewAPIServer(exec Executor) *APIServer {
	return &APIServer{exec}
}

type keyAccessor func(re *regexp.Regexp) []string

func buildGetResponseKeys(w http.ResponseWriter, r *http.Request, keysFunc keyAccessor, keysLen, responseItems int) []string {
	var keys []string

	if pattern := r.URL.Query().Get("pattern"); pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return nil
		}
		keys = keysFunc(re)
	}

	if v := r.URL.Query().Get("count"); v != "" {
		var length int
		if keys != nil {
			length = len(keys)
		} else {
			length = keysLen
		}
		js, err := json.Marshal(&length)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return nil
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
		return nil
	}

	if keys == nil {
		keys = keysFunc(nil)
	}
	sort.Strings(keys)

	// select cursor window
	var start int
	count := responseItems
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		var err error
		start, err = strconv.Atoi(startStr)
		if err != nil {
			http.Error(w, "Invalid value for query parameter \"start\"", http.StatusBadRequest)
			return nil
		}
		if start >= len(keys) {
			http.Error(w, "\"start\" index must be smaller than the number of objects", http.StatusBadGateway)
			return nil
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
			count = v
		}
	}
	if count > len(keys)-start {
		count = len(keys) - start
	}

	w.Header().Set("Content-Type", "application/json")
	return keys[:count]
}

func (svc *APIServer) getPipeline(w http.ResponseWriter, r *http.Request) {
	elements := strings.Split(r.URL.Path, "/")
	pipeName := elements[len(elements)-1]
	if len(elements) < 2 || elements[len(elements)-2] != "pipeline" {
		http.Error(w, r.URL.Path, http.StatusNotFound)
		return
	}

	if pipeline := svc.exec.PipelineLookup(pipeName); pipeline != nil {
		js, err := json.Marshal(pipeline)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} else {
		http.Error(w, pipeName, http.StatusNotFound)
	}
}

// reload the pipeline configuration
func (svc *APIServer) putPipeline(w http.ResponseWriter, r *http.Request) {
	elements := strings.Split(r.URL.Path, "/")
	pipeName := elements[len(elements)-1]
	if len(elements) < 2 || elements[len(elements)-2] != "pipeline" {
		http.Error(w, r.URL.Path, http.StatusNotFound)
		return
	}

	if pipeline := svc.exec.PipelineLookup(pipeName); pipeline != nil {
		if pipeline.State != StateStopped {
			http.Error(w, fmt.Sprintf("Pipeline reload in invalid state: %s", pipeline.State), http.StatusBadRequest)
			return
		}
		if err := svc.exec.PipelineReload(pipeline); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	} else {
		http.Error(w, pipeName, http.StatusNotFound)
	}
}

func (svc *APIServer) deletePipeline(w http.ResponseWriter, r *http.Request) {
	elements := strings.Split(r.URL.Path, "/")
	pipeName := elements[len(elements)-1]

	var request DeleteRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p := svc.exec.PipelineLookup(pipeName)
	if p == nil {
		http.Error(w, pipeName, http.StatusNotFound)
	}
	if request.InstanceID != 0 {
		if instance := p.getInstance(request.InstanceID); instance == nil {
			http.Error(w, fmt.Sprintf("invalid instance %d", request.InstanceID), http.StatusNotFound)
			return
		}
		svc.exec.DeleteInstance(p, request.InstanceID)
	} else {
		if len(p.Instances) > 0 {
			http.Error(w, "Pipeline has instances", http.StatusBadRequest)
			return
		}
		svc.exec.PipelineDelete(p)
	}
}

func (svc *APIServer) getPipelines(w http.ResponseWriter, r *http.Request) {
	keys := buildGetResponseKeys(w, r, svc.exec.PipelineMapKeys, svc.exec.PipelineCount(), 128)
	if keys == nil {
		return
	}
	result := make([]*Pipeline, len(keys))
	for i, k := range keys {
		result[i] = svc.exec.PipelineLookup(k)
	}
	js, err := json.Marshal(&result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(js)
}

func (svc *APIServer) postPipelines(w http.ResponseWriter, r *http.Request) {
	var request PipelinesPostRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if pipeline := svc.exec.PipelineLookup(request.Name); pipeline != nil {
		http.Error(w, fmt.Sprintf("pipeline %s already present", request.Name), http.StatusConflict)
	}

	if err := svc.exec.PipelineAdd(request.Name, request.URI); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (svc *APIServer) putState(w http.ResponseWriter, r *http.Request) {
	elements := strings.Split(r.URL.Path, "/")
	pipeName := elements[len(elements)-1]

	pipeline := svc.exec.PipelineLookup(pipeName)
	if pipeline == nil {
		http.Error(w, pipeName, http.StatusNotFound)
		return
	}

	var request StateRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch request.Action {
	case ActionStart:
		if request.ID > 0 {
			instance := pipeline.getInstance(request.ID)
			if instance == nil {
				http.Error(w, fmt.Sprintf("invalid instance id: %d", request.ID), http.StatusBadRequest)
				return
			}
			if instance.State == StateRunning {
				http.Error(w, fmt.Sprintf("Invalid state for start operation: %s", instance.State), http.StatusBadRequest)
				return
			}
		}
	case ActionStop:

	default:
		http.Error(w, string(request.Action), http.StatusBadRequest)
		return
	}

	if request.Stage >= len(pipeline.Config.Spec.Tasks) {
		http.Error(w, fmt.Sprintf("invalid stage id %d", request.Stage), http.StatusBadRequest)
		return
	}

	if err := svc.exec.SetState(pipeline, request.Action, request.ID, request.Stage); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// CloneRequest defines the json API for the clone endpoint
type CloneRequest struct {
	Pipeline string `json:"pipeline"`
	Instance int    `json:"instance"`
	Include  string `json:"include"`
	Exclude  string `json:"exclude"`
}

func (svc *APIServer) putClone(w http.ResponseWriter, r *http.Request) {
	var request CloneRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pipeline := svc.exec.PipelineLookup(request.Pipeline)
	if pipeline == nil {
		http.Error(w, request.Pipeline, http.StatusNotFound)
		return
	}

	if err := svc.exec.Clone(pipeline, request.Instance, request.Include, request.Exclude); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (svc *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dest := r.URL.Path[len(APIServerURLPath):]
	elements := strings.SplitN(dest, "/", 2)
	switch r.Method {
	case http.MethodGet:
		switch elements[0] {
		case "pipeline":
			svc.getPipeline(w, r)
		case "pipelines":
			svc.getPipelines(w, r)
		default:
			http.NotFound(w, r)
		}
	case http.MethodPost:
		switch dest {
		case "pipelines":
			svc.postPipelines(w, r)
		default:
			http.NotFound(w, r)
		}
	case http.MethodPut:
		switch elements[0] {
		case "pipeline":
			svc.putPipeline(w, r)
		case "state":
			svc.putState(w, r)
		case "clone":
			svc.putClone(w, r)
		default:
			http.NotFound(w, r)
		}
	case http.MethodDelete:
		switch elements[0] {
		case "pipeline":
			svc.deletePipeline(w, r)
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
