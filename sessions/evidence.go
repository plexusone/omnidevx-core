package sessions

import "time"

// Evidence is what a session touched, derived from the structured tool
// records in its transcript and from git, not from text the model wrote.
//
// It is a lower bound. A harness records file reads and edits made through
// its own tools, but shell commands can change files without leaving a path
// to extract, so a repository or file missing from Evidence was not observed,
// not proven untouched.
type Evidence struct {
	// Repos are the repositories the session worked in, read, modified, or mentioned.
	Repos []RepoActivity `json:"repos,omitempty"`
	// Files are the files the session read or modified through the harness's own tools.
	Files []FileActivity `json:"files,omitempty"`
	// Commits are the commits the session created or referenced.
	Commits []CommitRef `json:"commits,omitempty"`
	// WorkRefs are work-tracking identifiers, such as initiative and roadmap-item IDs,
	// found in the session.
	WorkRefs []WorkRef `json:"workRefs,omitempty"`
}

// RepoActivity says how a session relates to one repository. The flags are
// independent: a repository can be the starting directory, be read, and be
// modified at once.
type RepoActivity struct {
	// Root is the repository's root directory.
	Root string `json:"root"`
	// ID identifies the repository, such as github.com/org/repo, derived from its origin
	// remote. It is empty when the repository has no remote.
	ID string `json:"id,omitempty"`
	// CWD is set when the session started in this repository.
	CWD bool `json:"cwd,omitempty"`
	// Executed is set when a command ran with this repository as its working directory.
	Executed bool `json:"executed,omitempty"`
	// Read is set when the session read a file in this repository through a harness tool.
	Read bool `json:"read,omitempty"`
	// Modified is set when the session wrote or edited a file in this repository.
	Modified bool `json:"modified,omitempty"`
	// Mentioned is set when the repository was named in a prompt but not otherwise observed.
	Mentioned bool `json:"mentioned,omitempty"`
	// FileCount is the number of distinct files read or modified in this repository.
	FileCount int `json:"fileCount,omitempty"`
	// FirstSeen is the earliest observed activity in this repository.
	FirstSeen time.Time `json:"firstSeen,omitzero"`
	// LastSeen is the latest observed activity in this repository.
	LastSeen time.Time `json:"lastSeen,omitzero"`
}

// FileActivity says whether a session read or modified one file.
type FileActivity struct {
	// Path is the absolute path of the file.
	Path string `json:"path"`
	// Read is set when the session read the file through a harness tool.
	Read bool `json:"read,omitempty"`
	// Modified is set when the session wrote or edited the file.
	Modified bool `json:"modified,omitempty"`
	// LastSeen is when the session last touched the file.
	LastSeen time.Time `json:"lastSeen,omitzero"`
}

// CommitRelation says how a session relates to a commit.
type CommitRelation string

// Commit relations.
const (
	// CommitCreated means the session ran the command that made the commit.
	CommitCreated CommitRelation = "created"
	// CommitReferenced means a prompt named the commit, for example to review it.
	CommitReferenced CommitRelation = "referenced"
)

// CommitRef is a commit a session created or referenced.
type CommitRef struct {
	// SHA is the commit hash as the session saw it, abbreviated or full.
	SHA string `json:"sha"`
	// Repo is the root of the repository that holds the commit, when it could be resolved.
	Repo string `json:"repo,omitempty"`
	// Relation is created or referenced.
	Relation CommitRelation `json:"relation"`
	// Subject is the commit's first message line, when it could be read from git.
	Subject string `json:"subject,omitempty"`
	// At is when the session created or referenced the commit.
	At time.Time `json:"at,omitzero"`
}

// WorkRefSource says where a work reference was found.
type WorkRefSource string

// Work reference sources.
const (
	// WorkRefPrompt means a prompt a person typed.
	WorkRefPrompt WorkRefSource = "prompt"
	// WorkRefBranch means the session's git branch name.
	WorkRefBranch WorkRefSource = "branch"
	// WorkRefCommit means the message of a commit the session created.
	WorkRefCommit WorkRefSource = "commit"
	// WorkRefPath means a path the session touched.
	WorkRefPath WorkRefSource = "path"
)

// WorkRef is a work-tracking identifier found in a session.
type WorkRef struct {
	// ID is the identifier as written, such as RMI-EXAMPLE-012.
	ID string `json:"id"`
	// Kind is the name of the rule that matched it, such as initiative or rmi.
	Kind string `json:"kind"`
	// Sources lists where it was found.
	Sources []WorkRefSource `json:"sources"`
	// Title is the identifier's title when a resolver supplied one.
	Title string `json:"title,omitempty"`
}
