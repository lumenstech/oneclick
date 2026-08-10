package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// StablePlanHash returns the authorization-grade hash used by the CLI and
// DeployLocal. Analyzer fallback service names historically used the checkout
// directory basename, which is not stable across temporary GitHub workspaces.
// Normalize only that fallback class; explicit Compose/Ollama/vLLM service
// identities remain part of the hash.
func StablePlanHash(p Plan) string {
	p.PlanHash = ""
	for i := range p.Services {
		switch p.Services[i].Name {
		case "compose-stack", "ollama", "vllm":
			// Explicit analyzer identities are already stable.
		default:
			p.Services[i].Name = p.App.Name
		}
	}
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
