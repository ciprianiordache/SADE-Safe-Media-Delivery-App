// Package brand is an operator's own look: company name, logo, and how the
// watermark sits on their files (position, opacity, size, default text). One
// row per account. Each upload freezes a snapshot of it on the job (see
// internals/wmopts), so the worker, the recipient's page and the
// preview-ready email all show the brand as it was when the file was sent.
package brand

import "time"

// Defaults for an account that has not set anything yet.
const (
	DefaultPosition     = "bottom-right"
	DefaultOpacityPct   = 40
	DefaultLogoScalePct = 18
	DefaultTextTemplate = "{company} · {recipient} · {date}"
)

// Brand is one account's brand settings.
type Brand struct {
	ID           string    `db:"id,primary_key,uuid"`
	UserID       string    `db:"user_id,notnull,unique,references:users(id),on_delete:cascade"`
	CompanyName  string    `db:"company_name"`
	LogoKey      string    `db:"logo_key"` // storage key of the normalised PNG; "" = none
	Position     string    `db:"position"`
	OpacityPct   int       `db:"opacity_pct"`
	LogoScalePct int       `db:"logo_scale_pct"`
	TextTemplate string    `db:"text_template"`
	Onboarded    int       `db:"onboarded"` // 1 once the welcome setup was finished or skipped
	CreatedAt    time.Time `db:"created_at,oncreate"`
	UpdatedAt    time.Time `db:"updated_at,onwrite"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Brand) TableName() string { return "brands" }

// UpdateRequest is PUT /api/brand's body.
type UpdateRequest struct {
	CompanyName  string `json:"companyName"`
	Position     string `json:"position"`
	OpacityPct   int    `json:"opacityPct"`
	LogoScalePct int    `json:"logoScalePct"`
	TextTemplate string `json:"textTemplate"`
}

// Response is the API view.
type Response struct {
	CompanyName  string `json:"companyName"`
	HasLogo      bool   `json:"hasLogo"`
	LogoVersion  string `json:"logoVersion,omitempty"` // changes with the logo - a cache-buster
	Position     string `json:"position"`
	OpacityPct   int    `json:"opacityPct"`
	LogoScalePct int    `json:"logoScalePct"`
	TextTemplate string `json:"textTemplate"`
	Onboarded    bool   `json:"onboarded"`
}

func defaults(userID string) Brand {
	return Brand{
		UserID: userID, Position: DefaultPosition, OpacityPct: DefaultOpacityPct,
		LogoScalePct: DefaultLogoScalePct, TextTemplate: DefaultTextTemplate,
	}
}

func toResponse(b Brand) Response {
	r := Response{
		CompanyName: b.CompanyName, HasLogo: b.LogoKey != "", Position: b.Position,
		OpacityPct: b.OpacityPct, LogoScalePct: b.LogoScalePct, TextTemplate: b.TextTemplate,
		Onboarded: b.Onboarded == 1,
	}
	if b.LogoKey != "" {
		// brands/<user>/<hash>.png -> <hash>
		k := b.LogoKey
		for i := len(k) - 1; i >= 0; i-- {
			if k[i] == '/' {
				k = k[i+1:]
				break
			}
		}
		if len(k) > 4 {
			r.LogoVersion = k[:len(k)-4]
		}
	}
	return r
}
