package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v3"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
)

var (
	path_prefix = "/status/capacity"
)

func handleSigterm(cancel func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	klog.Info("Received SIGTERM. Terminating...")
	cancel()
}

type informerFactory interface {
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool
}

func waitForCacheSync(ctx context.Context, factory informerFactory) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for typ, done := range factory.WaitForCacheSync(ctx.Done()) {
		if !done {
			select {
			case <-ctx.Done():
				return fmt.Errorf("failed to sync %v: %v", typ, ctx.Err())
			default:
				return fmt.Errorf("failed to sync %v", typ)
			}
		}
	}
	return nil
}

type patchUInt32Value struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value uint32 `json:"value"`
}

/*
	type Node struct {
		name     string
		ipAddr   string
		hostName string
		ref      *core_v1.Node
	}
*/
func createPatch(resource string, value int) ([]byte, error) {
	payload := []patchUInt32Value{{
		Op:    "add",
		Path:  fmt.Sprintf("%s/%s", path_prefix, strings.Replace(resource, "/", "~1", -1)),
		Value: uint32(value),
	}}

	patch, err := json.Marshal(payload)
	return patch, err
}

func patchNodeCapacity(c *kubernetes.Clientset, nodeName string, payloadBytes []byte) (err error) {
	// TODO CHECK IF NOT ALREADY PATCHED
	node, err := c.CoreV1().Nodes().Patch(context.TODO(), nodeName, types.JSONPatchType, payloadBytes, meta_v1.PatchOptions{}, "status")
	if err != nil {
		klog.Infof("FAILED Patching node %s %v", nodeName, err)
		return err
	}
	klog.Infof("Patched node %s capacity: %v", nodeName, node.Status.Capacity)
	return nil
}

func getNodeResoures(nodeLabels map[string]string, conf *ConfigOpts, nodeCopacityConf map[string]string) (int, error) {
	if asg, ok := nodeLabels[conf.LabelAsg]; ok { //label `kops.k8s.io/instancegroup: taintest`
		if _, ok := nodeCopacityConf[asg]; ok {
			if capacity, ok := nodeCopacityConf[asg]; ok {
				return strconv.Atoi(capacity)
			}
		}
	}
	// TODO sprintf more verbose error nodename etc
	return 0, errors.New("node not matched by labels or config")
}

func patchNode(node *core_v1.Node, client *kubernetes.Clientset, conf *ConfigOpts, nodeCopacityConf map[string]string) {
	nodeName := node.GetName()
	nodeLabels := node.GetLabels()

	/*
		if labelHostname, ok := labels["kubernetes.io/hostname"]; ok {
			klog.Infof("Event nodeName: %s labelHostname: %s", nodeName, labelHostname)
		}
	*/
	// node.Status.Addresses
	// [{Type:InternalIP Address:172.26.91.234} {Type:Hostname Address:ip-172-26-91-234.eu-west-1.compute.internal}
	// {Type:InternalDNS Address:ip-172-26-91-234.eu-west-1.compute.internal}]
	klog.Infof("Processing nodeName: %s", nodeName)
	if !nodeutil.IsNodeReady(node) {
		klog.Infof("Node %s NOTREADY", nodeName)
		return
	}

	capacity, err := getNodeResoures(nodeLabels, conf, nodeCopacityConf)
	if err != nil {
		klog.Warningf("Cannot get resources for node %s cause: %s", nodeName, err)
		return
	}
	if capacity < 1 {
		klog.Infof("Zero capacity node %s, should never happen, please double CHECK config")
		return
	}
	patch, err := createPatch(conf.ResourceName, capacity)
	if err != nil {
		klog.Error("Create patch failed")
		return
	}
	if conf.DryRun {
		klog.Infof("DRY RUN patch node %s with %v", nodeName, patch)
		return
	}
	alreadyPatched := false
	if gotCapacity, ok := node.Status.Capacity[core_v1.ResourceName(conf.ResourceName)]; ok {
		klog.Infof("Node %s already has %s value ", nodeName, conf.ResourceName, gotCapacity)
		alreadyPatched = true
	}
	if !alreadyPatched || conf.Force {
		err = patchNodeCapacity(client, nodeName, patch)
		klog.Infof("Patching node %s with %s : %d", nodeName, conf.ResourceName, capacity)
		if err != nil {
			klog.Infof("FAILED Patching node %s %v", nodeName, err)
			return
		}
	}
}

func main() {
	klog.InitFlags(nil)
	conf := &ConfigOpts{}
	parser := flags.NewParser(conf, flags.Default)
	if _, err := parser.Parse(); err != nil {
		klog.Fatalf("Error parsing flags: %v", err)
	}

	data, err := os.ReadFile(conf.ConfigFilePath)
	if err != nil {
		klog.Fatalf("Cannot read config file %v", err)
	}
	var nodeCopacityConf map[string]string

	if err = yaml.Unmarshal([]byte(data), &nodeCopacityConf); err != nil {
		klog.Fatalf("Cannot parse config file %v", err)
	}

	client, err := createK8sClient(conf.KubeConfig)
	if err != nil {
		klog.Fatalf("NO kubeconfig", err)
	}
	if conf.DryRun {
		klog.Infof("Runing in DRY-RUN mode")
	}
	klog.Infof("Client %+v", client)
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()

	nodeInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(nodeObj interface{}) {
				node := nodeObj.(*core_v1.Node)
				patchNode(node, client, conf, nodeCopacityConf)
			},
			UpdateFunc: func(oldNodeObj, newNodeObj interface{}) {
				//oldNode := oldNodeObj.(*core_v1.Node)
				newNode := newNodeObj.(*core_v1.Node)
				klog.Infof("Update Event node %s", newNode.GetName())
				patchNode(newNode, client, conf, nodeCopacityConf)
			},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSigterm(cancel)
	informerFactory.Start(ctx.Done())

	if err := waitForCacheSync(context.Background(), informerFactory); err != nil {
		klog.Fatalf("Unable to sync", err)
	}

	<-ctx.Done()
}
