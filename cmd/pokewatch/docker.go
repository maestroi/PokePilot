package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

// dockerClient is the few Engine API calls the watcher needs, over the
// manager's unix socket. No SDK: three GETs and one service update.
type dockerClient struct {
	http *http.Client
	base string
}

func newDockerClient(socket string) *dockerClient {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &dockerClient{http: &http.Client{Transport: tr, Timeout: 30 * time.Second}, base: "http://docker/v1.43"}
}

type rollbackEvent struct {
	Service string
	At      time.Time
}

func (d *dockerClient) get(path string, out any) error {
	res, err := d.http.Get(d.base + path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("docker GET %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (d *dockerClient) Nodes() ([]operatorapi.SwarmNode, error) {
	var raw []struct {
		Description   struct{ Hostname string }
		Spec          struct{ Role string }
		Status        struct{ State string }
		ManagerStatus *struct {
			Reachability string
			Leader       bool
		}
	}
	if err := d.get("/nodes", &raw); err != nil {
		return nil, err
	}
	out := make([]operatorapi.SwarmNode, 0, len(raw))
	for _, n := range raw {
		sn := operatorapi.SwarmNode{Hostname: n.Description.Hostname, Manager: n.Spec.Role == "manager", Status: n.Status.State}
		if n.ManagerStatus != nil {
			sn.ManagerStatus = n.ManagerStatus.Reachability
			if n.ManagerStatus.Leader {
				sn.ManagerStatus = "leader"
			}
		}
		out = append(out, sn)
	}
	return out, nil
}

// Services lists the stacks' services with replica counts and any rollback
// the Swarm performed (one event per service and completion time).
func (d *dockerClient) Services(stacks []string) ([]operatorapi.ServiceState, []rollbackEvent, error) {
	var raw []struct {
		ID      string
		Version struct{ Index uint64 }
		Spec    struct {
			Name   string
			Labels map[string]string
		}
		ServiceStatus *struct{ RunningTasks, DesiredTasks int }
		UpdateStatus  *struct {
			State       string
			CompletedAt time.Time
		}
	}
	if err := d.get("/services?status=true", &raw); err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, s := range stacks {
		want[s] = true
	}
	var out []operatorapi.ServiceState
	var rolls []rollbackEvent
	for _, s := range raw {
		if !want[s.Spec.Labels["com.docker.stack.namespace"]] {
			continue
		}
		st := operatorapi.ServiceState{Name: s.Spec.Name}
		if s.ServiceStatus != nil {
			st.Running, st.Desired = s.ServiceStatus.RunningTasks, s.ServiceStatus.DesiredTasks
		}
		if s.UpdateStatus != nil {
			st.UpdateState = s.UpdateStatus.State
			if strings.HasPrefix(s.UpdateStatus.State, "rollback") {
				rolls = append(rolls, rollbackEvent{Service: s.Spec.Name, At: s.UpdateStatus.CompletedAt})
			}
		}
		out = append(out, st)
	}
	return out, rolls, nil
}

// ServiceLabel reads one label of a service ("" when unset).
func (d *dockerClient) ServiceLabel(name, key string) (string, error) {
	var svc struct {
		Spec struct{ Labels map[string]string }
	}
	if err := d.get("/services/"+url.PathEscape(name), &svc); err != nil {
		return "", err
	}
	return svc.Spec.Labels[key], nil
}

// SetServiceLabel sets (or with value "" removes) one service label. The
// spec is posted back unchanged otherwise, so tasks are not restarted:
// labels on the service spec do not touch the task template.
func (d *dockerClient) SetServiceLabel(name, key, value string) error {
	var svc struct {
		Version struct{ Index uint64 }
		Spec    map[string]any
	}
	if err := d.get("/services/"+url.PathEscape(name), &svc); err != nil {
		return err
	}
	labels, _ := svc.Spec["Labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
	}
	if value == "" {
		delete(labels, key)
	} else {
		labels[key] = value
	}
	svc.Spec["Labels"] = labels
	body, _ := json.Marshal(svc.Spec)
	res, err := d.http.Post(fmt.Sprintf("%s/services/%s/update?version=%d", d.base, url.PathEscape(name), svc.Version.Index), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("docker service update %s: %s", name, res.Status)
	}
	return nil
}
