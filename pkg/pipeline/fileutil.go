package pipeline

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/iterator"

	"cloud.google.com/go/storage"
)

const (
	fileScheme          = "file://"
	googleStorageScheme = "gs://"
)

func newGCloudReader(uri string) (io.ReadCloser, error) {
	path := uri[len(googleStorageScheme):]
	elements := strings.SplitN(path, "/", 2)
	if len(elements) < 2 {
		return nil, fmt.Errorf("invalid google storage uri: %s", uri)
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	bkt := client.Bucket(elements[0])
	obj := bkt.Object(elements[1])

	ctx, _ = context.WithTimeout(ctx, time.Minute)
	return obj.NewReader(ctx)
}

func newGCloudWriter(uri string) (io.WriteCloser, error) {
	path := uri[len(googleStorageScheme):]
	elements := strings.SplitN(path, "/", 2)
	if len(elements) < 2 {
		return nil, fmt.Errorf("invalid google storage uri: %s", uri)
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	bkt := client.Bucket(elements[0])
	obj := bkt.Object(elements[1])

	ctx, _ = context.WithTimeout(ctx, time.Minute)
	return obj.NewWriter(ctx), nil
}

// newFileReader allocates a file reader. It is the responsibility of the caller to
// call Close().
func newFileReader(uri string) (io.ReadCloser, error) {
	switch {
	case strings.HasPrefix(uri, googleStorageScheme):
		return newGCloudReader(uri)
	case strings.HasPrefix(uri, fileScheme):
		return os.Open(uri[len(fileScheme):])
	}
	return nil, fmt.Errorf("unsupported uri scheme: %s", uri)
}

func newFileWriter(uri string) (io.WriteCloser, error) {
	switch {
	case strings.HasPrefix(uri, googleStorageScheme):
		return newGCloudWriter(uri)
	case strings.HasPrefix(uri, fileScheme):
		return os.Create(uri[len(fileScheme):])
	}
	return nil, fmt.Errorf("unsupported uri scheme: %s", uri)
}

func newLocalCopy(uri string, prefix string) (string, error) {
	rd, err := newFileReader(uri)
	if err != nil {
		return "", nil
	}
	defer rd.Close()

	var dir string
	if os.Getenv("POD_NAME") != "" {
		dir = "/data"
	}
	tmpFile, err := ioutil.TempFile(dir, prefix)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, rd)
	return tmpFile.Name(), err
}

func gsCopyWorkDir(src, dst string, reIncl, reExcl *regexp.Regexp) error {
	if !strings.HasPrefix(src, googleStorageScheme) {
		return fmt.Errorf("source directory (%s) not a gs uri", src)
	}
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	srcElements := strings.SplitN(src[len(googleStorageScheme):], "/", 2)
	if len(srcElements) < 2 {
		return fmt.Errorf("invalid google storage uri: %s", src)
	}

	dstElements := strings.SplitN(dst[len(googleStorageScheme):], "/", 2)
	if len(dstElements) < 2 {
		return fmt.Errorf("invalid google storage uri: %s", dst)
	}

	srcBucket := client.Bucket(srcElements[0])
	var dstBucket *storage.BucketHandle
	if dstElements[0] != srcElements[0] {
		dstBucket = client.Bucket(dstElements[0])
	} else {
		dstBucket = srcBucket
	}

	q := &storage.Query{Prefix: srcElements[1]}
	iter := srcBucket.Objects(ctx, q)
	for {
		obj, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil
		}
		name := obj.Name[len(srcElements[1]):]
		if name[0] == '/' {
			name = name[1:]
		}

		if reIncl != nil && !reIncl.MatchString(name) {
			continue
		}
		if reExcl != nil && reExcl.MatchString(name) {
			continue
		}
		s := srcBucket.Object(obj.Name)
		d := dstBucket.Object(filepath.Join(dstElements[1], name))
		copier := d.CopierFrom(s)
		if _, err := copier.Run(ctx); err != nil {
			log.Println(err)
		}
	}

	return nil
}

func copyWorkDir(src, dst string, reIncl, reExcl *regexp.Regexp) error {
	switch {
	case strings.HasPrefix(dst, googleStorageScheme):
		return gsCopyWorkDir(src, dst, reIncl, reExcl)
	default:
		return fmt.Errorf("unsupported uri scheme: %s", dst)
	}
}
