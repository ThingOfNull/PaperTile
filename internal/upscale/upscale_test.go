package upscale

import (
	"image/color"
	"testing"

	xi "github.com/disintegration/imaging"
)

func TestValidateCropSize(t *testing.T) {
	if err := ValidateCropSize(4000, 1000); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCropSize(5000, 1250); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCropSize(4000, 998); err == nil {
		t.Fatal("expected aspect ratio error")
	}
	if err := ValidateCropSize(5001, 1300); err == nil {
		t.Fatal("expected error for long side > 5000")
	}
	if err := ValidateCropSize(100, 9); err == nil {
		t.Fatal("expected error for short side < 10")
	}
}

func TestEncodeJPEGUnder(t *testing.T) {
	img := xi.New(800, 600, color.RGBA{A: 255})
	b, err := EncodeJPEGUnder(img, MaxJPEGBytesBeforeBase64, 80, 92)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 || len(b) > MaxJPEGBytesBeforeBase64 {
		t.Fatalf("bad jpeg size %d", len(b))
	}
}
