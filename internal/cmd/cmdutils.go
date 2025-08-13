package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FatalError(l logr.Logger, err error, msg string, fields ...interface{}) {
	l.Error(err, msg, fields...)
	os.Exit(1)
}

func GetEnvOrFatalError(name string, logger logr.Logger) string {
	val := os.Getenv(name)
	if val == "" {
		FatalError(logger, errors.New("empty value"), "Could not get the environment variable", "name", name)
	}

	return val
}

func GetOperatorReplicaSet(ctx context.Context, client client.Client, namespace string) (*appsv1.ReplicaSet, error) {
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		return nil, fmt.Errorf("HOSTNAME environment variable not set")
	}

	pod := &corev1.Pod{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: namespace,
	}, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to get current pod %s: %v", podName, err)
	}

	var replicaSetName string
	for _, ownerRef := range pod.GetOwnerReferences() {
		if ownerRef.Kind == "ReplicaSet" && ownerRef.APIVersion == "apps/v1" {
			replicaSetName = ownerRef.Name
			break
		}
	}

	if replicaSetName == "" {
		return nil, fmt.Errorf("pod %s is not owned by a ReplicaSet", podName)
	}

	replicaSet := &appsv1.ReplicaSet{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      replicaSetName,
		Namespace: namespace,
	}, replicaSet)
	if err != nil {
		return nil, fmt.Errorf("failed to get ReplicaSet %s: %v", replicaSetName, err)
	}

	return replicaSet, nil
}
