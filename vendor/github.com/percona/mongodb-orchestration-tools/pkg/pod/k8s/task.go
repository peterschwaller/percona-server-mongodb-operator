// Copyright 2018 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"errors"
	"strings"

	"github.com/percona/mongodb-orchestration-tools/pkg/db"
	"github.com/percona/mongodb-orchestration-tools/pkg/pod"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	mongodContainerName     string = "mongod"
	mongosContainerName     string = "mongos"
	mongodbPortName         string = "mongodb"
	clusterServiceDNSSuffix string = "svc.cluster.local"
)

func GetMongoHost(pod, service, replset, namespace string) string {
	return strings.Join([]string{pod, service + "-" + replset, namespace, clusterServiceDNSSuffix}, ".")
}

type TaskState struct {
	status corev1.PodStatus
}

func NewTaskState(status corev1.PodStatus) *TaskState {
	return &TaskState{status}
}

func (ts TaskState) String() string {
	return strings.ToUpper(string(ts.status.Phase))
}

type Task struct {
	pod         *corev1.Pod
	statefulset *appsv1.StatefulSet
	serviceName string
	namespace   string
}

func NewTask(pod *corev1.Pod, statefulset *appsv1.StatefulSet, serviceName, namespace string) *Task {
	return &Task{
		pod:         pod,
		statefulset: statefulset,
		namespace:   namespace,
		serviceName: serviceName,
	}
}

func (t *Task) State() pod.TaskState {
	return NewTaskState(t.pod.Status)
}

func (t *Task) HasState() bool {
	return t.pod.Status.Phase != ""
}

func (t *Task) Name() string {
	return t.pod.Name
}

func (t *Task) IsRunning() bool {
	if t.pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, container := range t.pod.Status.ContainerStatuses {
		if container.State.Running == nil {
			return false
		}
	}
	return true
}

func (t *Task) IsUpdating() bool {
	if t.statefulset == nil {
		return false
	}
	status := t.statefulset.Status
	if status.CurrentRevision != status.UpdateRevision {
		return true
	}
	return status.ReadyReplicas != status.CurrentReplicas
}

func (t *Task) IsTaskType(taskType pod.TaskType) bool {
	var containerName string
	switch taskType {
	case pod.TaskTypeMongod:
		containerName = mongodContainerName
	case pod.TaskTypeMongos:
		containerName = mongosContainerName
	default:
		return false
	}
	for _, container := range t.pod.Spec.Containers {
		if container.Name == containerName {
			return true
		}
	}
	return false
}

func (t *Task) GetMongoAddr() (*db.Addr, error) {
	for _, container := range t.pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name != mongodbPortName {
				continue
			}
			replset, err := t.GetMongoReplsetName()
			if err != nil {
				return nil, err
			}
			addr := &db.Addr{
				Host: GetMongoHost(t.pod.Name, t.serviceName, replset, t.namespace),
				Port: int(port.HostPort),
			}
			if addr.Port == 0 {
				addr.Port = int(port.ContainerPort)
			}
			return addr, nil
		}
	}
	return nil, errors.New("could not find mongodb address")
}

func (t *Task) GetMongoReplsetName() (string, error) {
	return getPodReplsetName(t.pod)
}
