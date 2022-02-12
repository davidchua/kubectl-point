package cmd

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	networkingv1apply "k8s.io/client-go/applyconfigurations/networking/v1"

	"github.com/manifoldco/promptui"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/client/clientset/versioned"

	validation "k8s.io/apimachinery/pkg/util/validation"
)

var (
	to             string
	selectedIssuer string
	tlsAuto        bool
	namespace      string
	kubeconfig     *string
	rootCmd        = &cobra.Command{
		Use:   "point [domain] --to=[ip:port]",
		Short: "Headless Point",
		Long:  "\n\nThis command creates an ingress that when accessed, forwards the connection to an external resource transparently. Example:\n\n\tpoint example.org --to=172.169.1.4[:3000]\n\tpoint https://example.org --to=newdomain.com",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			// rely on KUBECONFIG envvar
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules,
				configOverrides)

			log.SetFlags(0)
			if namespace == "" {
				var err error
				log.Println("No namespace flag inserted, defaulting to kubeconfig's context namespace")
				namespace, _, err = kubeConfig.Namespace()
				if err != nil {
					log.Fatalf("❌ unable to get config namespace: %#v", err)
				}
			}

			config, err := kubeConfig.ClientConfig()
			if err != nil {
				log.Fatalf("❌ unable to get clientconfig: %#v", err)
			}

			splitTo := strings.Split(to, ":")
			if len(splitTo) != 2 {
				log.Fatalf("❌ expecting -to flag to be in the format: <ip/host>:<port> but got %#v instead", to)
			}

			client, err := kubernetes.NewForConfig(config)
			if err != nil {
				log.Fatalf("❌ error creating client: %#v", err)
			}

			subject := args[0]
			sanitized, err := sanitize(subject)
			if err != nil {
				log.Fatalf("❌ error sanitizing domain name: %#v", err)
			}

			// create Service
			ip := splitTo[0]
			p, err := strconv.Atoi(splitTo[1])
			if err != nil {
				log.Fatalf("❌ error converting port arguments into: %#v", p)
			}

			port := int32(p)
			portStr := splitTo[1]
			portProtocol := corev1.Protocol("TCP")

			if tlsAuto {

				ctx := context.Background()
				cmanager, err := certmanagerv1.NewForConfig(config)
				if err != nil {
					log.Fatal(err)
				}

				rr := cmanager.CertmanagerV1().ClusterIssuers()
				list, err := rr.List(ctx, metav1.ListOptions{})
				if err != nil {
					log.Fatal("❌ selected tls-auto option but unable to find any cluster-issuers or issuers, check that your cluster and that you're using cert-manager v1 and above")
				}

				var items []string
				for _, v := range list.Items {
					items = append(items, v.Name)
				}

				ilist, err := cmanager.CertmanagerV1().Issuers("").List(ctx, metav1.ListOptions{})
				for _, v := range ilist.Items {
					items = append(items, v.Name)

				}

				if len(items) > 0 {
					prompt := promptui.Select{
						Label: "Select clusterissuer",
						Items: items,
					}

					_, result, err := prompt.Run()
					if err != nil {
						log.Fatalf("❌ error getting prompt for cert-manager issuers: %#v", err)
					}
					selectedIssuer = result
				} else {
					log.Fatal("❌ selected tls-auto option but unable to find any cluster-issuers or issuers, check if you have any cluster-issuers or issuers on your cluster")
					selectedIssuer = ""

				}

			}

			apiversionStr := "v1"
			kindStr := "Service"
			log.Printf("🚀 Deploying assets to namespace %q", namespace)
			log.Printf("👉 Pointing %s to host: %s and port: %s", subject, ip, portStr)

			service := &corev1apply.ServiceApplyConfiguration{
				TypeMetaApplyConfiguration: metav1apply.TypeMetaApplyConfiguration{Kind: &kindStr, APIVersion: &apiversionStr},
				ObjectMetaApplyConfiguration: &metav1apply.ObjectMetaApplyConfiguration{
					Name:      &sanitized,
					Namespace: &namespace,
					Labels: map[string]string{
						"link": sanitized,
					},
					Annotations: map[string]string{},
				},
				Spec: &corev1apply.ServiceSpecApplyConfiguration{
					Ports: []corev1apply.ServicePortApplyConfiguration{
						corev1apply.ServicePortApplyConfiguration{
							Name:     &portStr,
							Protocol: &portProtocol,
							Port:     &port,
						},
					},
				},
			}

			_, err = client.CoreV1().Services(namespace).Apply(context.TODO(), service, metav1.ApplyOptions{FieldManager: "point"})
			if err != nil {
				log.Fatalf("error creating service: %#v", err)
			}

			log.Println("🎁 Service successfully applied")

			// Create Endpoint
			kindStr = "Endpoints"
			endpoint := &corev1apply.EndpointsApplyConfiguration{
				TypeMetaApplyConfiguration: metav1apply.TypeMetaApplyConfiguration{Kind: &kindStr, APIVersion: &apiversionStr},
				ObjectMetaApplyConfiguration: &metav1apply.ObjectMetaApplyConfiguration{
					Name:      &sanitized,
					Namespace: &namespace,
					Labels: map[string]string{
						"link": sanitized,
					},
					Annotations: map[string]string{},
				},

				Subsets: []corev1apply.EndpointSubsetApplyConfiguration{
					corev1apply.EndpointSubsetApplyConfiguration{
						Addresses: []corev1apply.EndpointAddressApplyConfiguration{
							corev1apply.EndpointAddressApplyConfiguration{IP: &ip},
						},
						Ports: []corev1apply.EndpointPortApplyConfiguration{
							corev1apply.EndpointPortApplyConfiguration{Name: &portStr, Port: &port},
						},
					},
				},
			}

			_, err = client.CoreV1().Endpoints(namespace).Apply(context.TODO(), endpoint, metav1.ApplyOptions{FieldManager: "point"})
			if err != nil {
				endpointStatusError := err.(*errors.StatusError)
				log.Fatalf("❌ error creating endpoint: %s", endpointStatusError.ErrStatus.Message)
			}

			log.Printf("🎁 Endpoint successfully applied")

			pathType := networkingv1.PathTypeExact
			defaultPath := "/"

			kindStr = "Ingress"
			apiversionStr = "networking.k8s.io/v1"

			ingress := &networkingv1apply.IngressApplyConfiguration{
				TypeMetaApplyConfiguration: metav1apply.TypeMetaApplyConfiguration{Kind: &kindStr, APIVersion: &apiversionStr},
				ObjectMetaApplyConfiguration: &metav1apply.ObjectMetaApplyConfiguration{
					Name:      &sanitized,
					Namespace: &namespace,
				},
				Spec: &networkingv1apply.IngressSpecApplyConfiguration{
					Rules: []networkingv1apply.IngressRuleApplyConfiguration{
						networkingv1apply.IngressRuleApplyConfiguration{
							Host: &subject,
							IngressRuleValueApplyConfiguration: networkingv1apply.IngressRuleValueApplyConfiguration{
								HTTP: &networkingv1apply.HTTPIngressRuleValueApplyConfiguration{
									Paths: []networkingv1apply.HTTPIngressPathApplyConfiguration{
										networkingv1apply.HTTPIngressPathApplyConfiguration{
											Path:     &defaultPath,
											PathType: &pathType,
											Backend: &networkingv1apply.IngressBackendApplyConfiguration{
												Service: &networkingv1apply.IngressServiceBackendApplyConfiguration{
													Name: &sanitized,
													Port: &networkingv1apply.ServiceBackendPortApplyConfiguration{
														Number: &port,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			// check if --tls=auto is on. if it is, set it to use cert-manager
			if tlsAuto && selectedIssuer != "" {
				ingress.Annotations = make(map[string]string)
				ingress.Annotations["cert-manager.io/cluster-issuer"] = selectedIssuer
				sub := fmt.Sprintf("%s-tls", subject)
				tlsSpec := networkingv1apply.IngressTLSApplyConfiguration{
					Hosts:      []string{sub},
					SecretName: &sub,
				}
				ingress.Spec.TLS = []networkingv1apply.IngressTLSApplyConfiguration{tlsSpec}

			}

			_, err = client.NetworkingV1().Ingresses(namespace).Apply(context.TODO(), ingress, metav1.ApplyOptions{FieldManager: "point"})

			if err != nil {
				log.Fatalf("❌ error creating ingress: %#v", err)
			}

			fmt.Printf("❤ Ingress successfully applied, you can now access it from \033[1;36mhttp://%s\n", args[0])
		},
	}
)

// sanitize takes in a domain name and converts it into a parsable Kubernetes
// object label. Sanitization is needed as we will be using the returned output
// as the various Service/Ingress object names and k8s requires that the first
// character has to be an alphabet and that "." becomes hyphens.
func sanitize(subject string) (string, error) {
	// Object names must be following RFC 1035 naming convention
	if v := validation.IsFullyQualifiedDomainName(nil, subject); v != nil {
		return "", fmt.Errorf("domain subject not valid domain %#v", v)
	}

	hostname := strings.Replace(subject, ".", "-", -1)
	validateRFC1123 := validation.IsDNS1035Label(hostname)
	if validateRFC1123 != nil {
		// if validation fails here, it can be assumed that its failing due to the
		// domain name starting with numeric value
		return "rootdomain-" + hostname, nil
	}

	return hostname, nil

}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&to, "to", "", "address to point to in a host:ip format (eg. 10.0.100.10:8080)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Namespace where you want the ingress/service to be created. if unassigned, uses current-context's namespace")
	rootCmd.PersistentFlags().BoolVar(&tlsAuto, "tls-auto", true, "Set true if want to use cert-manager to automatically assign a tls cert")
}
