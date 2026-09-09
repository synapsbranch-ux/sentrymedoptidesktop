package server

import (
	"fmt"
	"net/http"
	"testing"
)

func (a *testApp) sellTo(t *testing.T, patientID string, amount int64) (invoiceID, itemID string) {
	t.Helper()
	item := a.createInventoryItem(map[string]any{"sku": fmt.Sprintf("SVC-%d", amount), "category": "service", "name": "Comprehensive exam", "salePriceMinor": amount, "currency": "HTG", "trackStock": false})
	created := a.request(http.MethodPost, "/api/v1/invoices", map[string]any{
		"patientId": patientID, "currency": "HTG",
		"items": []map[string]any{{"inventoryItemId": item, "description": "Comprehensive exam", "quantity": 1, "unitPriceMinor": amount}},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("invoice: %d %s", created.Code, created.Body.String())
	}
	invoiceID = decodeResponse[map[string]any](t, created)["id"].(string)
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	items, _ := detail["items"].([]any)
	if len(items) > 0 {
		line, _ := items[0].(map[string]any)
		if id, ok := line["id"].(string); ok {
			itemID = id
		}
	}
	return invoiceID, itemID
}

func TestAClaimIsProposedFromThePolicysCoverage(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Covered", "Patient")
	policy := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "Assurance Sante", "policyNumber": "P-4471", "memberNumber": "M-99", "coveragePercent": 70, "isPrimary": true,
	}, a.doctor)
	if policy.Code != http.StatusCreated {
		t.Fatalf("record policy: %d %s", policy.Code, policy.Body.String())
	}
	invoiceID, _ := a.sellTo(t, patient.ID, 200000)

	proposal := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/claims/proposal?patientId="+patient.ID+"&invoiceId="+invoiceID, nil, a.doctor))
	if proposal["policyFound"] != true {
		t.Fatalf("the patient's policy was not found: %v", proposal)
	}
	// 70% of 200000 to the insurer, the rest to the patient.
	if proposal["payerPortionMinor"] != float64(140000) || proposal["patientPortionMinor"] != float64(60000) {
		t.Fatalf("the split does not follow the policy's coverage: %v", proposal)
	}
	if policyDetail, _ := proposal["policy"].(map[string]any); policyDetail["policyNumber"] != "P-4471" {
		t.Fatalf("the proposal lost the policy details: %v", proposal["policy"])
	}
}

func TestAProposalDoesNotOfferWhatIsAlreadyClaimed(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Partly", "Claimed")
	_ = a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{"payerName": "Assurance", "coveragePercent": 50, "isPrimary": true}, a.doctor)
	invoiceID, _ := a.sellTo(t, patient.ID, 100000)
	payer := a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "Assurance"}, a.doctor)
	if payer.Code != http.StatusCreated {
		t.Fatalf("payer: %d %s", payer.Code, payer.Body.String())
	}
	payerID := decodeResponse[map[string]any](t, payer)["id"].(string)

	claim := a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 40000, "payerPortionMinor": 20000, "patientPortionMinor": 20000,
	}, a.doctor)
	if claim.Code != http.StatusCreated {
		t.Fatalf("claim: %d %s", claim.Code, claim.Body.String())
	}

	proposal := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/claims/proposal?patientId="+patient.ID+"&invoiceId="+invoiceID, nil, a.doctor))
	if proposal["alreadyClaimedMinor"] != float64(40000) || proposal["claimAmountMinor"] != float64(60000) {
		t.Fatalf("the proposal offers to claim money already claimed: %v", proposal)
	}
}

func TestAProposalWithNoPolicySaysSoRatherThanGuessing(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Uninsured", "Patient")
	invoiceID, _ := a.sellTo(t, patient.ID, 50000)
	proposal := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/claims/proposal?patientId="+patient.ID+"&invoiceId="+invoiceID, nil, a.doctor))
	if proposal["policyFound"] != false || proposal["payerPortionMinor"] != float64(0) {
		t.Fatalf("a patient with no policy was given a split anyway: %v", proposal)
	}
	if proposal["patientPortionMinor"] != float64(50000) {
		t.Fatalf("the whole bill should fall to an uninsured patient: %v", proposal)
	}
}

func TestAPolicyNeedsARealCoverageFigure(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Bad", "Policy")
	for _, body := range []map[string]any{
		{"payerName": "", "coveragePercent": 50},
		{"payerName": "Assurance", "coveragePercent": 140},
		{"payerName": "Assurance", "coveragePercent": -1},
	} {
		if response := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", body, a.doctor); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v = %d, want 422", body, response.Code)
		}
	}
}

func TestAProposalRefusesSomebodyElsesInvoice(t *testing.T) {
	a := newTestApp(t)
	owner := a.createPatient(a.doctor, "Invoice", "Owner")
	other := a.createPatient(a.doctor, "Someone", "Different")
	invoiceID, _ := a.sellTo(t, owner.ID, 10000)
	response := a.request(http.MethodGet, "/api/v1/insurance/claims/proposal?patientId="+other.ID+"&invoiceId="+invoiceID, nil, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-patient proposal = %d, want 422", response.Code)
	}
}

func TestAClaimCanCoverASingleInvoiceLine(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Line", "Claim")
	invoiceID, itemID := a.sellTo(t, patient.ID, 80000)
	if itemID == "" {
		t.Skip("the invoice detail does not expose line ids")
	}
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "Assurance"}, a.doctor))["id"].(string)
	claim := a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID, "invoiceItemId": itemID,
		"claimAmountMinor": 80000, "payerPortionMinor": 80000, "patientPortionMinor": 0,
	}, a.doctor)
	if claim.Code != http.StatusCreated {
		t.Fatalf("line-level claim: %d %s", claim.Code, claim.Body.String())
	}
	mismatched := a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID, "invoiceItemId": "not-on-this-invoice",
		"claimAmountMinor": 1000, "payerPortionMinor": 1000, "patientPortionMinor": 0,
	}, a.doctor)
	if mismatched.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a claim for a line on another invoice = %d, want 422", mismatched.Code)
	}
}
