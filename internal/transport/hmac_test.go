package transport

import "testing"

func TestSignVerify(t *testing.T) {
	body := Body{
		Modules: []ModuleData{
			{Module: "system", Metrics: []MetricPoint{{Name: "cpu", Value: 42.0}}},
		},
	}
	secret := "test-secret-key"

	sig, err := Sign(body, secret)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if err := Verify(body, secret, sig); err != nil {
		t.Fatalf("Verify with correct secret: %v", err)
	}

	if err := Verify(body, "wrong-secret", sig); err == nil {
		t.Fatal("Verify with wrong secret should fail")
	}
}

func TestSignDeterministic(t *testing.T) {
	body := Body{
		Modules: []ModuleData{
			{Module: "test", Metrics: []MetricPoint{{Name: "m", Value: 1.0}}},
		},
	}

	sig1, _ := Sign(body, "key")
	sig2, _ := Sign(body, "key")

	if sig1 != sig2 {
		t.Fatalf("signatures differ: %s vs %s", sig1, sig2)
	}
}
