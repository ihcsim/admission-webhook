package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	webhook "github.com/ihcsim/sidecar-injector"
	"github.com/ihcsim/sidecar-injector/test"
	"github.com/sirupsen/logrus"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
)

var testServer *WebhookServer

func TestMain(m *testing.M) {
	// mock out the k8s clientset constructor
	webhook.NewClient = test.NewFakeClient

	// create a webhook which uses its fake client to seed the sidecar configmap
	w, err := initWebhookWithConfigMap()
	if err != nil {
		panic(err)
	}

	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	logger := logrus.NewEntry(log)
	testServer = &WebhookServer{nil, w, logger}

	os.Exit(m.Run())
}

func TestServe(t *testing.T) {
	t.Run("With Empty HTTP Request Body", func(t *testing.T) {
		in := bytes.NewReader(nil)
		request := httptest.NewRequest(http.MethodGet, "/", in)

		recorder := httptest.NewRecorder()
		testServer.serve(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("HTTP response status mismatch. Expected: %d. Actual: %d", http.StatusOK, recorder.Code)
		}

		if reflect.DeepEqual(recorder.Body.Bytes(), []byte("")) {
			t.Errorf("Content mismatch. Expected HTTP response body to be empty %v", recorder.Body.Bytes())
		}
	})

	t.Run("With Valid HTTP Request Body", func(t *testing.T) {
		body, err := test.FixtureHTTPRequestBody("http-request-body-valid.json", "../..")
		if err != nil {
			t.Fatal("Unexpected error: ", err)
		}

		in := bytes.NewReader(body)
		request := httptest.NewRequest(http.MethodGet, "/", in)

		recorder := httptest.NewRecorder()
		testServer.serve(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("HTTP response status mismatch. Expected: %d. Actual: %d", http.StatusOK, recorder.Code)
		}

		expected, err := test.FixtureAdmissionReview("admission-review-request-response.json", "../..")
		if err != nil {
			t.Fatal("Unexpected error: ", err)
		}

		var actual admissionv1beta1.AdmissionReview
		if err := json.Unmarshal(recorder.Body.Bytes(), &actual); err != nil {
			t.Fatal("Unexpected error: ", err)
		}

		if !reflect.DeepEqual(actual, *expected) {
			t.Errorf("Content mismatch\nExpected: %+v\nActual: %+v", *expected, actual)
		}
	})
}

func TestHandleRequestError(t *testing.T) {
	var (
		errMsg   = "Some test error"
		recorder = httptest.NewRecorder()
		err      = fmt.Errorf(errMsg)
	)

	testServer.handleRequestError(recorder, err, http.StatusInternalServerError)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("HTTP response status mismatch. Expected: %d. Actual: %d", http.StatusInternalServerError, recorder.Code)
	}

	if strings.TrimSpace(recorder.Body.String()) != errMsg {
		t.Errorf("HTTP response body mismatch. Expected: %q. Actual: %q", errMsg, recorder.Body.String())
	}
}

func initWebhookWithConfigMap() (*webhook.Webhook, error) {
	fixture, err := webhook.New()
	if err != nil {
		return nil, err
	}

	// seed the sidecar configmap with the fake client
	configMap, err := test.FixtureConfigMap("../..", "sidecar-configmap.json")
	if err != nil {
		return nil, err
	}

	if _, err := fixture.Client.CoreV1().ConfigMaps(test.DefaultNamespace).Create(configMap); err != nil {
		return nil, err
	}

	return fixture, nil
}
