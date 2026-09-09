package server

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A real, decodable PNG rather than a magic-number stub, so the stored bytes are
// something a browser would actually render.
func samplePNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 4, 4))
	canvas.Set(1, 1, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func (a *testApp) uploadImage(t *testing.T, entityType, entityID, filename string, content []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	_ = form.WriteField("entityType", entityType)
	_ = form.WriteField("entityId", entityID)
	_ = form.WriteField("caption", "Front view")
	file, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/images", body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	return a.requestRaw(request, cookie)
}

func TestAFrameCarriesItsPhotograph(t *testing.T) {
	a := newTestApp(t)
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-IMG", "category": "frame", "name": "Photographed frame", "salePriceMinor": 100000, "currency": "HTG", "quantity": 2, "trackStock": true})

	uploaded := a.uploadImage(t, "inventory_item", itemID, "frame.png", samplePNG(t), a.doctor)
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", uploaded.Code, uploaded.Body.String())
	}
	record := decodeResponse[map[string]any](t, uploaded)
	if record["mediaType"] != "image/png" || record["caption"] != "Front view" {
		t.Fatalf("the stored image does not describe itself: %v", record)
	}

	listed := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/images?entityType=inventory_item&entityId="+itemID, nil, a.nurse))
	items, _ := listed["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("the frame shows %d images", len(items))
	}

	imageID := record["id"].(string)
	content := a.request(http.MethodGet, "/api/v1/images/"+imageID+"/content", nil, a.nurse)
	if content.Code != http.StatusOK || content.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image content: %d %s", content.Code, content.Header().Get("Content-Type"))
	}
	if content.Body.Len() == 0 {
		t.Fatal("the stored image is empty")
	}

	if removed := a.request(http.MethodDelete, "/api/v1/images/"+imageID, nil, a.doctor); removed.Code != http.StatusOK {
		t.Fatalf("delete image: %d", removed.Code)
	}
	after := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/images?entityType=inventory_item&entityId="+itemID, nil, a.doctor))
	if items, _ := after["items"].([]any); len(items) != 0 {
		t.Fatalf("a deleted image is still listed: %v", items)
	}
}

func TestAnImageIsJudgedByItsBytesNotItsName(t *testing.T) {
	a := newTestApp(t)
	itemID := a.createInventoryItem(map[string]any{"sku": "FR-BAD", "category": "frame", "name": "Frame", "salePriceMinor": 1000, "currency": "HTG", "quantity": 1, "trackStock": true})
	// A script renamed .png is refused: the type comes from the content.
	response := a.uploadImage(t, "inventory_item", itemID, "frame.png", []byte("#!/bin/sh\nrm -rf /\n"), a.doctor)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("a renamed script = %d, want 415: %s", response.Code, response.Body.String())
	}
}

func TestAnImageNeedsSomethingRealToBelongTo(t *testing.T) {
	a := newTestApp(t)
	for _, testCase := range []struct{ entityType, entityID string }{
		{"inventory_item", "does-not-exist"},
		{"patient", "anything"},
		{"inventory_item", ""},
	} {
		response := a.uploadImage(t, testCase.entityType, testCase.entityID, "frame.png", samplePNG(t), a.doctor)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("attaching to %q/%q = %d, want 422", testCase.entityType, testCase.entityID, response.Code)
		}
	}
}

func TestALabOrderCarriesReferenceImages(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Lab", "Images")
	created := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{"patientId": patient.ID, "lensType": "single_vision", "notes": "Frame supplied by patient"}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create lab order: %d %s", created.Code, created.Body.String())
	}
	orderID := decodeResponse[map[string]any](t, created)["id"].(string)

	if response := a.uploadImage(t, "lab_order", orderID, "frame.png", samplePNG(t), a.nurse); response.Code != http.StatusCreated {
		t.Fatalf("attach a reference photo to a lab order: %d %s", response.Code, response.Body.String())
	}
	listed := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/images?entityType=lab_order&entityId="+orderID, nil, a.doctor))
	if items, _ := listed["items"].([]any); len(items) != 1 {
		t.Fatalf("the lab order shows %d images", len(items))
	}
}
