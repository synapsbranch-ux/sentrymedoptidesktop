package clinicalvalues

import (
	"math"
	"testing"
)

func TestNumberAcceptsClinicFormatsAndRejectsMissing(t *testing.T) {
	tests := []struct {
		input any
		want  float64
		ok    bool
	}{
		{"−2,50 D", -2.5, true},
		{"17 mmHg", 17, true},
		{"510 µm", 510, true},
		{"", 0, false},
		{"not measured", 0, false},
		{nil, 0, false},
	}
	for _, test := range tests {
		got, ok := Number(test.input)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("Number(%v)=(%v,%v), want (%v,%v)", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestSnellenToLogMAR(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"20/20", 0}, {"20/40-2", 0.3}, {"6/12", 0.3}, {"0,5", 0.3}, {"CF", 1.9}, {"NLP", 3.0},
	}
	for _, test := range tests {
		got, ok := SnellenToLogMAR(test.input)
		if !ok || math.Abs(got-test.want) > 0.01 {
			t.Errorf("SnellenToLogMAR(%q)=(%v,%v), want %v", test.input, got, ok, test.want)
		}
	}
}

func TestRefractionSourcesPreserveSphereCylinderAndAxis(t *testing.T) {
	combined := RefractionFromCombined(`{"odSphere":"-4.00","odCylinder":"-1.00","odAxis":"180","osSphere":"-2.00"}`, "subjective")
	if combined == nil || combined.OD.Sphere == nil || *combined.OD.Sphere != -4 || combined.OD.SphericalEquivalent == nil || *combined.OD.SphericalEquivalent != -4.5 || combined.OD.Axis == nil || *combined.OD.Axis != 180 {
		t.Fatalf("combined refraction = %#v", combined)
	}
	separate := RefractionFromEyes(`{"odsphere":"-3.00","odcylinder":"-0.50","odaxis":"90"}`, `{"sphere":"-1.00","cylinder":"-1.00","axis":"45"}`, "prescription")
	if separate == nil || separate.OD.SphericalEquivalent == nil || *separate.OD.SphericalEquivalent != -3.25 || separate.OS.SphericalEquivalent == nil || *separate.OS.SphericalEquivalent != -1.5 {
		t.Fatalf("separate refraction = %#v", separate)
	}
}

func TestNormalizedMeasurementsKeepMissingValuesNil(t *testing.T) {
	acuity := VisualAcuity(`{"odBestCorrectedVA":"20/40","osUncorrectedDistance":"20/20"}`)
	if acuity == nil || acuity.OD == nil || *acuity.OD != 0.3 || acuity.OS == nil || *acuity.OS != 0 {
		t.Fatalf("acuity = %#v", acuity)
	}
	iop := IOP(`{"odIOP":"18","osIOP":""}`)
	if iop == nil || iop.OD == nil || *iop.OD != 18 || iop.OS != nil {
		t.Fatalf("iop = %#v", iop)
	}
	pachymetry := Pachymetry(`{"odThickness":"505 µm"}`)
	if pachymetry == nil || pachymetry.OD == nil || *pachymetry.OD != 505 || pachymetry.OS != nil {
		t.Fatalf("pachymetry = %#v", pachymetry)
	}
	keratometry := KeratometryValues(`{"odK1":"43.25","odK1 Axis":"90","osK2":"44,50"}`)
	if keratometry == nil || keratometry.OD.K1 == nil || *keratometry.OD.K1 != 43.25 || keratometry.OS.K2 == nil || *keratometry.OS.K2 != 44.5 {
		t.Fatalf("keratometry = %#v", keratometry)
	}
}
