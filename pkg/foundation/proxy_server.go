// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

const (
	// OCMProxyServerName is the name of the ocm proxy server deployment
	OCMProxyServerName   string = "ocm-proxyserver"
	KlusterletSecretName string = "ocm-klusterlet-self-signed-secrets" // #nosec G101 (not credentials)

	OCMProxyAPIServiceName               string = "v1beta1.proxy.open-cluster-management.io"
	OCMClusterViewV1APIServiceName       string = "v1.clusterview.open-cluster-management.io"
	OCMClusterViewV1alpha1APIServiceName string = "v1alpha1.clusterview.open-cluster-management.io"
	OCMProxyGroup                        string = "proxy.open-cluster-management.io"
	OCMClusterViewGroup                  string = "clusterview.open-cluster-management.io"
)

// OCMProxyServerDeployment creates the deployment for the ocm proxy server
func OCMProxyServerDeployment(m *backplanev1alpha1.BackplaneConfig, overrides map[string]string) *appsv1.Deployment {
	replicas := utils.GetReplicaCount()
	mode := int32(420)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OCMProxyServerName,
			Namespace: m.Namespace,
			Labels:    defaultLabels(OCMProxyServerName),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: defaultLabels(OCMProxyServerName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: defaultLabels(OCMProxyServerName),
				},
				Spec: corev1.PodSpec{
					// ImagePullSecrets:   []corev1.LocalObjectReference{{Name: m.Spec.ImagePullSecret}},
					ServiceAccountName: ServiceAccount,
					Tolerations:        defaultTolerations(),
					// NodeSelector:       m.Spec.NodeSelector,
					Affinity: utils.DistributePods("ocm-antiaffinity-selector", OCMProxyServerName),
					Volumes: []corev1.Volume{
						{
							Name: "klusterlet-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{DefaultMode: &mode, SecretName: KlusterletSecretName},
							},
						},
						{
							Name: "apiservice-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{DefaultMode: &mode, SecretName: OCMProxyServerName},
							},
						},
					},
					Containers: []corev1.Container{{
						Image:           Image(overrides),
						ImagePullPolicy: utils.GetImagePullPolicy(m),
						Name:            OCMProxyServerName,
						Args: []string{
							"/proxyserver",
							"--secure-port=6443",
							"--tls-cert-file=/var/run/apiservice/tls.crt",
							"--tls-private-key-file=/var/run/apiservice/tls.key",
							"--agent-cafile=/var/run/klusterlet/ca.crt",
							"--agent-certfile=/var/run/klusterlet/tls.crt",
							"--agent-keyfile=/var/run/klusterlet/tls.key",
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/healthz",
									Port:   intstr.FromInt(6443),
									Scheme: corev1.URISchemeHTTPS,
								},
							},
							InitialDelaySeconds: 2,
							PeriodSeconds:       10,
						},
						ReadinessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/healthz",
									Port:   intstr.FromInt(6443),
									Scheme: corev1.URISchemeHTTPS,
								},
							},
							InitialDelaySeconds: 2,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2048Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "klusterlet-certs", MountPath: "/var/run/klusterlet"},
							{Name: "apiservice-certs", MountPath: "/var/run/apiservice"},
						},
					}},
				},
			},
		},
	}

	dep.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(m, m.GetObjectKind().GroupVersionKind()),
	})
	return dep
}

// OCMProxyServerService creates a service object for the ocm proxy server
func OCMProxyServerService(m *backplanev1alpha1.BackplaneConfig) *corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OCMProxyServerName,
			Namespace: m.Namespace,
			Labels:    defaultLabels(OCMProxyServerName),
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": OCMProxyServerName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: defaultLabels(OCMProxyServerName),
			Ports: []corev1.ServicePort{{
				Name:       "secure",
				Protocol:   corev1.ProtocolTCP,
				Port:       443,
				TargetPort: intstr.FromInt(6443),
			}},
		},
	}

	s.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(m, m.GetObjectKind().GroupVersionKind()),
	})
	return s
}

// OCMProxyAPIService creates an apiservice object for the ocm proxy api
func OCMProxyAPIService(m *backplanev1alpha1.BackplaneConfig) *apiregistrationv1.APIService {
	s := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name: OCMProxyAPIServiceName,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
		Spec: apiregistrationv1.APIServiceSpec{
			Service: &apiregistrationv1.ServiceReference{
				Namespace: m.Namespace,
				Name:      OCMProxyServerName,
			},
			Group:                OCMProxyGroup,
			Version:              "v1beta1",
			GroupPriorityMinimum: 10000,
			VersionPriority:      20,
		},
	}

	return s
}

// OCMClusterViewV1APIService creates an apiservice object for the ocm clusterview api v1
func OCMClusterViewV1APIService(m *backplanev1alpha1.BackplaneConfig) *apiregistrationv1.APIService {
	s := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name: OCMClusterViewV1APIServiceName,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
		Spec: apiregistrationv1.APIServiceSpec{
			Service: &apiregistrationv1.ServiceReference{
				Namespace: m.Namespace,
				Name:      OCMProxyServerName,
			},
			Group:                OCMClusterViewGroup,
			Version:              "v1",
			GroupPriorityMinimum: 10,
			VersionPriority:      20,
		},
	}

	return s
}

// OCMClusterViewV1alpha1APIService creates an apiservice object for the ocm clusterview api V1alpha1
func OCMClusterViewV1alpha1APIService(m *backplanev1alpha1.BackplaneConfig) *apiregistrationv1.APIService {
	s := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name: OCMClusterViewV1alpha1APIServiceName,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
		Spec: apiregistrationv1.APIServiceSpec{
			Service: &apiregistrationv1.ServiceReference{
				Namespace: m.Namespace,
				Name:      OCMProxyServerName,
			},
			Group:                OCMClusterViewGroup,
			Version:              "v1alpha1",
			GroupPriorityMinimum: 10,
			VersionPriority:      20,
		},
	}

	return s
}