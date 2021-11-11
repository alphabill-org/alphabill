package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageName(t *testing.T) {
	pnr := &PackageNameResolver{BasePackage: "alphabill/alphabill", Depth: 1}
	assert.Equal(t, "internal/logger", pnr.PackageName())
}

func TestDepth(t *testing.T) {
	assert.Equal(t, 2, (&PackageNameResolver{}).depth(), "default should be 2")
	assert.Equal(t, 1, (&PackageNameResolver{Depth: 1}).depth())
	assert.Equal(t, 2, (&PackageNameResolver{Depth: 2}).depth())
	assert.Equal(t, 3, (&PackageNameResolver{Depth: 3}).depth())
}
