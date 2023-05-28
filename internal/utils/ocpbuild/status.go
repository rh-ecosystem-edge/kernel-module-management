package ocpbuild

type Status string

const (
	StatusCompleted  Status = "completed"
	StatusCreated    Status = "created"
	StatusInProgress Status = "in progress"
	StatusFailed     Status = "failed"
)
