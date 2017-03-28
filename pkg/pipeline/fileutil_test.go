package pipeline

import (
	"flag"
	"fmt"
	"regexp"
	"testing"
)

var (
	integration bool
)

func init() {
	flag.BoolVar(&integration, "integration", false, "Enable integration tests")
}

func TestLocalCopy(t *testing.T) {
	if !integration {
		t.Skip("skip integration test")
	}

	copy, err := newLocalCopy("gs://laserlike_roque/mr_sitedata/1/conf/token-allocator.yaml", "k8s")
	if err != nil {
		t.Error(err)
	}
	fmt.Println(copy)
}

func TestDirCopy(t *testing.T) {
	if !integration {
		t.Skip("skip integration test")
	}
	re, _ := regexp.Compile("of-00001")
	if err := copyWorkDir("gs://laserlike_roque/x", "gs://laserlike_roque/y", re, nil); err != nil {
		t.Error(err)
	}
}
