package school

import "DarkCS/entity"

type Core interface {
	AddSchools(names []string) ([]entity.School, error)
	GetSchools(status string) ([]entity.School, error)
	SetSchoolActive(name string, active bool) error
}
