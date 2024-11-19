package pod

import (
	"bytes"
	"context"
	"fmt"
	"net/url"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/util/podcmd"
)

type RemoteCommandExecutor interface {
	Run(pod *v1.Pod, cmd []string) ([]byte, []byte, error)
}

type remoteExecutor struct {
	*restclient.Config
}

func NewRemoteExecutor(config *restclient.Config) RemoteCommandExecutor {

	return &remoteExecutor{Config: config}
}

// executeRemoteCommand executes a remote shell command on the given namespaced pod
// returns the output from stdout and stderr

func (r *remoteExecutor) Run(pod *v1.Pod, cmd []string) ([]byte, []byte, error) {
	c := kubernetes.NewForConfigOrDie(r.Config)
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	container, err := podcmd.FindOrDefaultContainerByName(pod, "", true, errBuf)
	if err != nil {
		return nil, errBuf.Bytes(), err
	}
	containerName := container.Name
	request := c.CoreV1().RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		Param("container", containerName)

	request.VersionedParams(&v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}, scheme.ParameterCodec)
	exec, err := createExecutor(request.URL(), r.Config)
	// exec, err := remotecommand.NewSPDYExecutor(r.Config, "POST", request.URL())
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
		Tty:    false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed executing command %s on %s/%s: %v", cmd, pod.Namespace, pod.Name, err)
	}
	return buf.Bytes(), errBuf.Bytes(), nil
}

// createExecutor returns the Executor or an error if one occurred.
func createExecutor(url *url.URL, config *restclient.Config) (remotecommand.Executor, error) {
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", url)
	if err != nil {
		return nil, err
	}
	// Fallback executor is default, unless feature flag is explicitly disabled.
	if !cmdutil.RemoteCommandWebsockets.IsDisabled() {
		// WebSocketExecutor must be "GET" method as described in RFC 6455 Sec. 4.1 (page 17).
		websocketExec, err := remotecommand.NewWebSocketExecutor(config, "GET", url.String())
		if err != nil {
			return nil, err
		}
		exec, err = remotecommand.NewFallbackExecutor(websocketExec, exec, func(err error) bool {
			return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
		})
		if err != nil {
			return nil, err
		}
	}
	return exec, nil
}
