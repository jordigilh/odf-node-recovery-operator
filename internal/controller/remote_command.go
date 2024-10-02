package controller

import (
	"bytes"
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type remoteCommandExecutor interface {
	Run(podName, namespaceName string, cmd []string) (string, string, error)
}

type remoteExecutor struct {
	*rest.Config
}

func newRemoteExecutor(config *rest.Config) remoteCommandExecutor {

	return &remoteExecutor{Config: config}
}

// executeRemoteCommand executes a remote shell command on the given namespaced pod
// returns the output from stdout and stderr

func (r *remoteExecutor) Run(podName, namespaceName string, cmd []string) (string, string, error) {
	c := kubernetes.NewForConfigOrDie(r.Config)

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	request := c.RESTClient().
		Post().
		Namespace(namespaceName).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(r.Config, "POST", request.URL())
	ctx := context.Background()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})
	if err != nil {
		return "", "", fmt.Errorf("Failed executing command %s on %v/%v", cmd, namespaceName, podName)
	}

	return buf.String(), errBuf.String(), nil
}
