package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/pedro-r-marques/pipeline/pkg/pipeline"
)

var (
	dataDir       string
	httpStaticDir string
	httpPort      int
	jobConfigFile string
)

func init() {
	flag.StringVar(&dataDir, "data-dir", "/etc", "Directory for pipeline configuration")
	flag.StringVar(&httpStaticDir, "http-static-dir", "/var/www", "Directory for static web files")
	flag.IntVar(&httpPort, "port", 8080, "HTTP port")
	flag.StringVar(&jobConfigFile, "config", "file:///data/config.json", "Job configuration")
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/pipeline/static/index.html", http.StatusSeeOther)
}

func main() {
	flag.Parse()

	exec := pipeline.NewExecutor(dataDir)
	if jobConfigFile != "" {
		if err := exec.Configure(jobConfigFile); err != nil {
			log.Println(err)
		}
		exec.SetCheckpointFile(jobConfigFile)
	}
	exec.Start()

	srv := pipeline.NewAPIServer(exec)
	http.HandleFunc("/", redirectHandler)
	http.HandleFunc("/pipeline", redirectHandler)
	http.Handle(pipeline.APIServerURLPath, srv)
	http.Handle("/pipeline/static/", http.FileServer(http.Dir(httpStaticDir)))
	http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil)
}
