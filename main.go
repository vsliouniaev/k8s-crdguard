package main

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
)

var (
	deserializer = scheme.Codecs.UniversalDeserializer()
	client dynamic.Interface
)

func initK8s() {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.WithError(err).Fatal("error building kubernetes config")
	}

	c, err := dynamic.NewForConfig(config)
	if err != nil {
		log.WithError(err).Fatal("error creating kubernetes client")
	}
	client = c
}

func toAdmissionResponseFailure(message string) *v1beta1.AdmissionResponse {
	r := &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Details: &metav1.StatusDetails{
				Causes: []metav1.StatusCause{}}}}

	r.Result.Status = metav1.StatusFailure
	r.Result.Reason = metav1.StatusReasonInvalid
	r.Result.Code = http.StatusUnprocessableEntity
	r.Result.Message = message

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

func validateInstancesNotExists(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	if ar.Request.Resource.Group != "apiextensions.k8s.io" {
		err := fmt.Errorf("expected resource to be apiextensions.k8s.io, but received %v", ar.Request.Resource.Group)
		log.Error(err)
		return toAdmissionResponseFailure("Unexpected resource kind")
	}

	if ar.Request.Resource.Resource != "customresourcedefinitions" {
		err := fmt.Errorf("expected resource to be customresourcedefinitions, but received %v", ar.Request.Resource.Resource)
		log.Error(err)
		return toAdmissionResponseFailure("Unexpected resource kind")
	}

	var crd Crd
	if err := json.Unmarshal(ar.Request.OldObject.Raw, &crd); err != nil {
		log.WithError(err).WithField("oldObject", string(ar.Request.OldObject.Raw)).Error("cannot unmarshal CRD from spec")
		return toAdmissionResponseFailure("Cannot unmarshal CRD from spec")
	}

	s := crd.getOptimalSchema()
	list, err := client.Resource(s).List(metav1.ListOptions{Limit: 1})
	if err != nil {
		log.WithError(err).WithField("schema", s).Error("unable to get list of existing CRDs")
		return toAdmissionResponseFailure("Unable to get list of existing CRDs")
	}

	if len(list.Items) != 0 {
		log.Infof("found some %s, failing request", ar.Request.Name)
		return toAdmissionResponseFailure(fmt.Sprintf("There are still some %s in the cluster", ar.Request.Name))
	} else {
		log.Debug("found none, allowing delete")
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
		log.Warn("request has no body")
		http.Error(w, "request has no body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Warn(fmt.Sprintf("invalid Content-Type %s, want `application/json`", contentType))
		http.Error(w, "invalid Content-Type, want `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	log.WithField("content", string(body)).Debug("Received request")

	requestedAdmissionReview := v1beta1.AdmissionReview{}
	responseAdmissionReview := v1beta1.AdmissionReview{}

	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		log.WithError(err).Warn("unable to deserialize request")
		responseAdmissionReview.Response = toAdmissionResponseFailure("Unable to deserialize request")
	} else {
		responseAdmissionReview.Response = admit(requestedAdmissionReview)
	}

	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	respBytes, err := json.Marshal(responseAdmissionReview)

	log.WithField("content", string(respBytes)).Debug("sending response")

	if err != nil {
		log.WithError(err).Error("cannot serialize response")
		http.Error(w, fmt.Sprintf("could not serialize response: %v", err), http.StatusInternalServerError)
	}
	if _, err := w.Write(respBytes); err != nil {
		log.WithError(err).Error("cannot write response")
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK"))
	})
	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, validateInstancesNotExists)
	})
	initK8s()

	log.Info("Running on 8443")
	err := http.ListenAndServeTLS(":8443", "/cert/cert", "/cert/key", nil)
	if err != nil {
		log.Fatal(err)
	}
}
