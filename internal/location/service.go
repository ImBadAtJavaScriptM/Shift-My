package location

import (
	"fmt"
	"strings"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

type Service struct {
	store *storage.Store
}

func New(store *storage.Store) *Service {
	return &Service{store: store}
}

func (s *Service) SetTarget(lat, lon float64, label string) (storage.Installation, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return storage.Installation{}, fmt.Errorf("coordinates out of range")
	}
	return s.store.UpdateTarget(lat, lon, strings.TrimSpace(label))
}
