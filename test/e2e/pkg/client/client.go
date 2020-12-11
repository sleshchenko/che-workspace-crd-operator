//
// Copyright (c) 2019-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package client

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/devfile/api/pkg/apis/workspaces/v1alpha2"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"log"
	"os/exec"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Scheme             = runtime.NewScheme()
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
	SchemeGroupVersion = schema.GroupVersion{Group: v1alpha2.SchemeGroupVersion.Group, Version: v1alpha2.SchemeGroupVersion.Version}
)

func init() {
	if err := AddToScheme(scheme.Scheme); err != nil {
		logrus.Fatalf("Failed to add CRD to scheme")
	}
	if err := api.AddToScheme(Scheme); err != nil {
		logrus.Fatalf("Failed to add CRD to scheme")
	}
}

type K8sClient struct {
	kubeClient  *kubernetes.Clientset
	crClient    crclient.Client
	kubeCfgFile string // generate when client is created and store config there
}

// NewK8sClientWithKubeConfig creates kubernetes client wrapper with the specified kubeconfig file
func NewK8sClientWithKubeConfig(kubeconfigFile string) (*K8sClient, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	if err != nil {
		return nil, err
	}

	//TODO we need admins KUBECONFIG or credentials as arguments
	//TODO copy files with go
	cfgBump := fmt.Sprintf("/tmp/admin.%s.kubeconfig", generateUniqPrefixForFile())
	err = copyFile(kubeconfigFile, cfgBump)

	if err != nil {
		log.Fatal(fmt.Sprintf("Can't bump kubeconfig %s %s", err))
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	crClient, err := crclient.New(cfg, crclient.Options{})
	if err != nil {
		return nil, err
	}

	return &K8sClient{
		kubeClient:  client,
		crClient:    crClient,
		kubeCfgFile: cfgBump,
	}, nil
}

// NewK8sClientWithKubeConfig creates kubernetes client wrapper with the token
func NewK8sClientWithToken(token, clusterConsoleUrl string) (*K8sClient, error) {
	//TODO generate the suffix for the file
	cfgBump := fmt.Sprintf("/tmp/dev.%s.kubeconfig", generateUniqPrefixForFile())
	cmd := exec.Command("bash",
		"-c", fmt.Sprintf(
			"KUBECONFIG=%s"+
				" oc login --token %s --insecure-skip-tls-verify=true %s",
			cfgBump, token, clusterConsoleUrl))
	outBytes, err := cmd.CombinedOutput()
	output := string(outBytes)
	fmt.Println("Logged in with token as: " + output)
	cfg, err := clientcmd.BuildConfigFromFlags("", cfgBump)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	crClient, err := crclient.New(cfg, crclient.Options{})
	if err != nil {
		return nil, err
	}
	return &K8sClient{
		kubeClient:  client,
		crClient:    crClient,
		kubeCfgFile: cfgBump,
	}, nil
}

// Kube returns the clientset for Kubernetes upstream.
func (c *K8sClient) Kube() kubernetes.Interface {
	return c.kubeClient
}

//read a source file and copy to the selected path
func copyFile(sourceFile string, destinationFile string) error {
	input, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		fmt.Println(err)
		return err
	}

	err = ioutil.WriteFile(destinationFile, input, 0644)
	if err != nil {
		fmt.Println("Cannot create file  copy for config ", destinationFile)
		fmt.Println(err)
		return err
	}
	return nil
}

//generate unq prefix for using current time in milliseconds and get last 5 numbers
func generateUniqPrefixForFile() string {
	//get the uniq time in seconds as string
	prefix := strconv.FormatInt(int64(int(time.Now().UnixNano())), 10)
	//cut the string to last 5 uniq numbers
	prefix = prefix[14:len(prefix)]
	return prefix

}
