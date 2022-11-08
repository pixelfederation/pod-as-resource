package main

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	discovery "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func createK8sClient(kubeConfig string) (*kubernetes.Clientset, error) {
	// If neither masterUrl or kubeconfigPath are passed in we fallback to inClusterConfig.
	// If inClusterConfig fails, we fallback to the default config.
	k8sconf, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	k8sconf.UserAgent = "pod-as-resource"

	klog.InfoS("Creating API client", "host", k8sconf.Host)

	client, err := kubernetes.NewForConfig(k8sconf)
	if err != nil {
		return nil, err
	}

	var v *discovery.Info

	// The client may fail to connect to the API server in the first request.
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	klog.V(2).InfoS("Trying to discover Kubernetes version")
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		klog.V(2).ErrorS(err, "Unexpected error discovering Kubernetes version", "attempt", retries)
		retries++
		return false, nil
	})

	// err is returned in case of timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	// this should not happen, warn the user
	if retries > 0 {
		klog.Warningf("Initial connection to the Kubernetes API server was retried %d times.", retries)
	}

	klog.InfoS("Running in Kubernetes cluster",
		"major", v.Major,
		"minor", v.Minor,
		"git", v.GitVersion,
		"state", v.GitTreeState,
		"commit", v.GitCommit,
		"platform", v.Platform,
	)

	return client, nil
}
