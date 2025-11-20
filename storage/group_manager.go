package storage

type GroupManager interface {
	ListGroups() ([]string, error)
}
