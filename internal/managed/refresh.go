package managed

// Decision is what a refresh should do with one file.
type Decision int

const (
	DecisionUpdate        Decision = iota // managed & unchanged since last write -> safe to update
	DecisionKeepModified                  // managed but user edited -> keep unless --force
	DecisionKeepUserAdded                 // not in manifest -> user's own file, always keep
)

// DecideRefresh applies FR-021 to one file given its current on-disk content.
func (m *Manifest) DecideRefresh(filePath string, onDiskContent []byte) Decision {
	e, ok := m.Get(filePath)
	if !ok {
		return DecisionKeepUserAdded
	}
	if e.SHA256 == Fingerprint(onDiskContent) {
		return DecisionUpdate
	}
	return DecisionKeepModified
}
