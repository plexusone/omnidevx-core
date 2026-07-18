package git

import "strings"

// KnownAITool is an AI coding assistant recognizable by its co-author
// commit-trailer email.
type KnownAITool struct {
	// Name is the display name, e.g. "Claude Code".
	Name string
	// Emails are known co-author addresses. Entries of the form
	// "<id>+<slug>@users.noreply.github.com" also match any address with
	// the same "+<slug>@users.noreply.github.com" suffix.
	Emails []string
}

// KnownAITools lists recognized AI coding assistants and their co-author
// signatures. Ported from devfolio's contributor package, which this
// registry supersedes as the canonical copy.
var KnownAITools = []KnownAITool{
	{
		Name:   "Claude Code",
		Emails: []string{"noreply@anthropic.com"},
	},
	{
		Name:   "GitHub Copilot",
		Emails: []string{"noreply@github.com", "copilot@github.com"},
	},
	{
		Name: "Gemini CLI",
		Emails: []string{
			"218195315+gemini-cli@users.noreply.github.com",
			"176961590+gemini-code-assist[bot]@users.noreply.github.com",
			"gemini-cli-agent@google.com",
			"gemini@google.com",
		},
	},
	{
		Name:   "Cursor",
		Emails: []string{"ai@cursor.sh", "cursor@cursor.sh"},
	},
	{
		Name:   "Aider",
		Emails: []string{"aider@aider.chat"},
	},
}

// AIToolByEmail returns the AI tool matching a co-author email, or "" when
// the email is not a known AI signature. Matching is case-insensitive and
// supports GitHub noreply suffix forms with variable user-ID prefixes.
func AIToolByEmail(email string) string {
	email = strings.ToLower(email)
	for _, tool := range KnownAITools {
		for _, pattern := range tool.Emails {
			pattern = strings.ToLower(pattern)
			if email == pattern {
				return tool.Name
			}
			if plus := strings.Index(pattern, "+"); plus >= 0 &&
				strings.HasSuffix(pattern, "@users.noreply.github.com") {
				if strings.HasSuffix(email, pattern[plus:]) {
					return tool.Name
				}
			}
		}
	}
	return ""
}
