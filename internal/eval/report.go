package eval

import (
	"bytes"
	"html/template"
)

// Reporter renders a Report to HTML. Higher levels register a reporter for
// their level so the output matches the kind of change being evaluated (a
// charter A/B, a workflow A/B, a governance-policy A/B, ...).
type Reporter interface {
	Render(*Report) (string, error)
}

var reporters = map[string]Reporter{}

// RegisterReporter registers a reporter for an experiment level.
func RegisterReporter(level string, r Reporter) { reporters[level] = r }

// ReporterFor returns the reporter for a level, or the default HTML reporter.
func ReporterFor(level string) Reporter {
	if r, ok := reporters[level]; ok {
		return r
	}
	return HTMLReporter{}
}

// HTMLReporter renders a generic A/B HTML report.
type HTMLReporter struct{}

func (HTMLReporter) Render(r *Report) (string, error) {
	cmp, hasCmp := r.Compare()
	data := struct {
		*Report
		Cmp    Comparison
		HasCmp bool
	}{r, cmp, hasCmp}
	var b bytes.Buffer
	if err := htmlTmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func signClass(f float64) string {
	switch {
	case f > signEps:
		return "up"
	case f < -signEps:
		return "down"
	default:
		return "flat"
	}
}

var htmlTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"sign": signClass,
}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>eval: {{.Experiment}}</title>
<style>
 body{font:14px/1.5 system-ui,sans-serif;margin:2rem;color:#242424}
 h1{font-size:1.3rem} .lvl{color:#6a6a6a;font-weight:normal}
 table{border-collapse:collapse;margin:1rem 0} th,td{padding:.35rem .75rem;text-align:left;border-bottom:1px solid #eee}
 td.num{text-align:right;font-variant-numeric:tabular-nums}
 .up{color:#1a7f5a} .down{color:#c0392b} .flat{color:#6a6a6a}
 .verdict{padding:.5rem .75rem;background:#f6f6f6;border-radius:6px;display:inline-block}
</style></head><body>
<h1>eval: {{.Experiment}} {{if .Level}}<span class="lvl">· {{.Level}}</span>{{end}}</h1>

<h2>Variants</h2>
<table><tr><th>variant</th><th>mean</th></tr>
{{range .Variants}}<tr><td>{{.Name}}</td><td class="num">{{printf "%.2f" .Mean}}</td></tr>
{{end}}</table>

{{if .HasCmp}}
<h2>A/B — {{.Cmp.A}} → {{.Cmp.B}}</h2>
<table><tr><th>case</th><th>{{.Cmp.A}}</th><th>{{.Cmp.B}}</th><th>Δ</th></tr>
{{range .Cmp.Deltas}}<tr>
 <td>{{.Case}}</td><td class="num">{{printf "%.2f" .A}}</td><td class="num">{{printf "%.2f" .B}}</td>
 <td class="num {{sign .Diff}}">{{printf "%+.2f" .Diff}}</td></tr>
{{end}}
<tr><td><b>overall</b></td><td></td><td></td><td class="num {{sign .Cmp.Overall}}"><b>{{printf "%+.2f" .Cmp.Overall}}</b></td></tr>
</table>
<p class="verdict">{{.Cmp.Wins}} win / {{.Cmp.Losses}} loss / {{.Cmp.Ties}} tie ·
 sign-test p = {{printf "%.3f" .Cmp.P}}</p>
{{end}}
</body></html>
`))
