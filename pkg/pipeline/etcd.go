package pipeline

import (
	"context"
	"log"
	"time"

	"github.com/coreos/etcd/client"
)

type lockManager interface {
	DeleteLock(namespace, lockname string, id int) error
}

type etcdLockManager struct {
	client client.Client
	api    client.KeysAPI
}

// NewEtcdLockManager creates an etcd lock manager client
func NewEtcdLockManager(endpoint string) lockManager {
	cfg := client.Config{
		Endpoints: []string{endpoint},
		Transport: client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	kapi := client.NewKeysAPI(c)
	return &etcdLockManager{
		client: c,
		api:    kapi,
	}
}

func (m *etcdLockManager) DeleteLock(namespace, lockname string, id int) error {
	// client, err := locking.NewLockClientWithNamespace(fmt.Sprintf("%s-%d", lockname, id), namespace, "", 0)
	// if err != nil {
	// 	return err
	// }
	// return client.DeleteDir()
	path := lockname
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := m.api.Delete(ctx, path, &client.DeleteOptions{Recursive: true, Dir: true})
	return err
}
