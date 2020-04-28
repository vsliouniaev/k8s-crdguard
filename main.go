package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"net/http"
	"os"
)

var (
	deserializer   = scheme.Codecs.UniversalDeserializer()
	client         dynamic.Interface
	kubeconfigPath string
	explicitCrds   mapFlags = make(map[string]bool)
	log            logr.Logger
)

func toAdmissionResponseFailure(message string) *v1beta1.AdmissionResponse {
	r := &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{}}}}

	r.Result.Status = metav1.StatusFailure
	r.Result.Reason = metav1.StatusReasonInvalid
	r.Result.Code = http.StatusConflict
	r.Result.Message = message
	r.Result.Details = &metav1.StatusDetails{
		Causes: []metav1.StatusCause{{Message: message}},
	}

	return r
}

type admitFunc func(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

type Crd struct {
	Spec struct {
		Group    string `json:"group"`
		Versions []struct {
			Name    string `json:"name"`
			Served  bool   `json:"served"`
			Storage bool   `json:"storage"`
		} `json:"versions"`
		Names struct {
			Plural string `json:"plural"`
		} `json:"names"`
	} `json:"spec"`
}

func (c *Crd) getOptimalSchema() (o schema.GroupVersionResource) {
	o.Group = c.Spec.Group
	o.Resource = c.Spec.Names.Plural
	for v := range c.Spec.Versions {
		if c.Spec.Versions[v].Served {
			o.Version = c.Spec.Versions[v].Name
			if c.Spec.Versions[v].Storage {
				return
			}
		}
	}
	return
}

func shouldCheckCrd(ar v1beta1.AdmissionReview) bool {
	if len(explicitCrds) == 0 {
		return true
	}

	_, ok := explicitCrds[ar.Request.Name]
	return ok
}

func validateInstancesNotExists(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	if ar.Request.Resource.Group != "apiextensions.k8s.io" {
		log.Info("expected resource to be apiextensions.k8s.io", "actual", ar.Request.Resource.Group)
		return toAdmissionResponseFailure("Unexpected resource kind")
	}

	if ar.Request.Resource.Resource != "customresourcedefinitions" {
		log.Info("expected resource to be customresourcedefinitions", "actual", ar.Request.Resource.Resource)
		return toAdmissionResponseFailure("Unexpected resource kind")
	}

	if shouldCheckCrd(ar) {
		var crd Crd
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &crd); err != nil {
			log.Error(err, "cannot unmarshal CRD from spec", "oldObject", string(ar.Request.OldObject.Raw))
			return toAdmissionResponseFailure("Cannot unmarshal CRD from spec")
		}

		s := crd.getOptimalSchema()
		list, err := client.Resource(s).List(metav1.ListOptions{Limit: 1})
		if err != nil {
			log.Error(err, "unable to get list of existing CRDs", "schema", s)
			return toAdmissionResponseFailure("Unable to get list of existing CRDs")
		}

		if len(list.Items) != 0 {
			log.V(1).Info("at least on resource exists", "name", ar.Request.Name)
			return toAdmissionResponseFailure(fmt.Sprintf("There are still some %s in the cluster", ar.Request.Name))
		} else {
			log.V(1).Info("found none, allowing delete", "name", ar.Request.Name)
		}
	}

	return &v1beta1.AdmissionResponse{Allowed: true}
}

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		log.Info("request has no body")
		http.Error(w, "request has no body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Info("invalid Content-Type expected `application/json`", "actual", contentType)
		http.Error(w, "invalid Content-Type, want `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	log.V(2).Info("received request", "content", string(body))

	requestedAdmissionReview := v1beta1.AdmissionReview{}
	responseAdmissionReview := v1beta1.AdmissionReview{}

	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		log.Error(err, "unable to deserialize request")
		responseAdmissionReview.Response = toAdmissionResponseFailure("Unable to deserialize request")
	} else {
		responseAdmissionReview.Response = admit(requestedAdmissionReview)
	}

	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	respBytes, err := json.Marshal(responseAdmissionReview)

	log.V(2).Info("sending response", "content", string(respBytes))

	if err != nil {
		log.Error(err, "could not serialize response")
		http.Error(w, fmt.Sprintf("could not serialize response: %v", err), http.StatusInternalServerError)
	}
	if _, err := w.Write(respBytes); err != nil {
		log.Error(err, "could not write response")
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func main() {

	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to kubeconfig file: e.g. ~/.kube/kind-config-kind")
	logDebug := *flag.Bool("log-debug", false, "Whether to enable debug log configuration")
	certFile := *flag.String("cert-file", "/cert/cert", "Path to certificate file to serve TLS")
	keyFile := *flag.String("key-file", "/cert/key", "Path to key file to serve TLS")
	flag.Var(&explicitCrds, "crds", "List of crds to block deletion of. Default will block all CRDs. e.g. 'prometheuses.monitoring.coreos.com'")
	flag.Parse()

	configLogging(logDebug)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK"))
	})
	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, validateInstancesNotExists)
	})
	initK8s()

	log.Info("Running on 8443")
	err := http.ListenAndServeTLS(":8443", certFile, keyFile, nil)
	if err != nil {
		log.Error(err, "failed to start server")
	}
}

func configLogging(dev bool) {
	var config zap.Config
	if dev {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}
	zapLog, err := config.Build()
	if err != nil {
		panic(err)
	}

	log = zapr.NewLogger(zapLog)

	// Throw away client-go logs
	f := &flag.FlagSet{}
	klog.InitFlags(f)
	f.Set("log_file", "/dev/null")
	f.Set("logtostderr", "false")
}

func initK8s() {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Error(err, "error building kubernetes config")
		os.Exit(1)
	}

	c, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Error(err, "error creating kubernetes client")
		os.Exit(1)
	}
	client = c
}

type mapFlags map[string]bool

func (i *mapFlags) String() string {
	return ""
}

func (i *mapFlags) Set(value string) error {
	(*i)[value] = true
	return nil
}
