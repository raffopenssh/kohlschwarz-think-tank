package jobs

import (
	"regexp"
	"strings"
)

// Posting is a normalised job / tender / consultancy record.
type Posting struct {
	URL      string
	Source   string
	Title    string
	Org      string
	Location string
	Snippet  string
	Lang     string
	Region   string
	Posted   string
	Deadline string
	Tender   bool // procurement notice: topic match alone is enough
}

// --- Tri-lingual keyword matcher (EN / DE / FR) ---------------------------

// topic: must relate to parks / protected areas / conservation.
var topicRe = regexp.MustCompile(`(?i)\b(national\s*parks?|nationalparks?|parcs?\s+nationa(l|ux)|protected\s+areas?|conserved\s+areas?|aires?\s+prot[eé]g[eé]es?|schutzgebiet\w*|naturpark\w*|biosph[aä]ren\w*|biosphere\s+reserve|game\s+reserve|wildlife\s+reserve|nature\s+reserve|r[eé]serve\s+(naturelle|de\s+faune)|forest\s+reserve|world\s+heritage|conservanc(y|ies)|conservation\s+area|landscape\s+(conservation|manager|programme)|transfrontier|kaza|tfca|wildlife\s+(authority|service|management)|park\s+(manager|director|warden|management|operations)|reserve\s+manager|concession\s+manager|conservateur|nationalparkverwaltung|umweltschutz|naturschutz\w*|biodiversit[yéä]\w*|conservation|ranger\w*|wildlife|faune|ecosystem|écosystème|natura\s*2000|iucn|wwf|african\s+parks|peace\s+parks|wildlife\s+conservation\s+society|wcs|frankfurt\s+zoological|fzs|noé|conservation\s+partnership|collaborative\s+management|gestion\s+d[eé]l[eé]gu[eé]e|forest(ry)?|wald\w*|for[eê]t\w*)\b`)

// seniority / consultancy: director-level roles or expert consultancies.
var seniorRe = regexp.MustCompile(`(?i)\b(director\w*|directeur|directrice|direktor\w*|direktion|leiter\w*|leitung|gesch[aä]ftsf[uü]hr\w*|vorstand|head\s+of|chief|ceo|chef\s+de|country\s+(director|manager|representative)|park\s+manager|manager\s+(of\s+)?(park|reserve|conservanc|landscape|protected)|general\s+manager|senior\s+(manager|advisor|adviser|expert|consultant|technical|specialist|programme|program)|principal|lead\w*|superintendent|warden|conservateur|team\s+leader|chef\s+d.équipe|key\s+expert|expert\s+(principal|senior|international)|consultant\w*|consultanc(y|ies)|consultance|consultation|berater\w*|beratung\w*|gutachter\w*|expertise|technical\s+assistance|assistance\s+technique|tender|call\s+for|appel\s+(d.offres?|à\s+candidatures?|à\s+manifestation)|ausschreibung\w*|vergabe\w*|procurement|request\s+for\s+proposals?|rfp|terms?\s+of\s+reference|tor\b|termes?\s+de\s+r[eé]f[eé]rence|management\s+plan|plan\s+(de\s+gestion|d.am[eé]nagement)|managementplan|evaluation|évaluation|evaluierung|mid-?term\s+review|revue\s+à\s+mi-parcours|mi-parcours|halbzeit\w*|review|feasibility|governance|gouvernance|ppp|public.private\s+partnership|partenariat\s+public.priv[eé]|collaborative\s+management|co-?management|gestion\s+d[eé]l[eé]gu[eé]e|delegated\s+management|advisor|adviser)\b`)

// negatives: clearly not what we want.
var negRe = regexp.MustCompile(`(?i)\b(intern(ship)?|praktik\w*|stagiaire|stage\b|volunteer|freiwillig\w*|b[eé]n[eé]vole|student|studierende|phd|doctoral|postdoc|apprentice|lehrling|trainee|assistant\b|assistent\w*|assistante?\b|receptionist|driver|chauffeur|cook|koch|k[oö]chin|housekeeping|cleaner|reinigung|security\s+guard|barista|waiter|kellner|technician\b|techniker|produktionsmitarbeiter|lagermitarbeiter|verk[aä]ufer|sales\b|marketing\s+intern|car\s+park|parking|parkhaus|parkplatz|parkraum|business\s+park|industrial\s+park|science\s+park|technology\s+park|office\s+park|theme\s+park|amusement|water\s+park|skate\s*park|trailer\s+park|park\s+ave|parkway|parkinson|state\s+parks?|county\s+parks?|city\s+parks?|regional\s+parks?|municipal|parks?\s+(and|&)\s+recreation|recreation\s+district|city\s+of|town\s+of|hotel|lodge|resort|safari\s+camp|golf|zoo|aquarium|professor|faculty|lecturer|fellowship|head\s+of\s+digital|software|it\s+manager|accountant|finance\s+manager|hr\s+manager|communications?\s+(manager|officer)|content|fundrais\w*|development\s+director|membership|retail|visitor\s+(services|centre|center)|imaging|pilot|gis|m&e|monitoring,?\s+evaluation)\b`)

// tenderNegRe: works / supplies tenders that merely mention a park.
var tenderNegRe = regexp.MustCompile(`(?i)\b(construction|bau\w*|travaux|roads?|stra[sß]e\w*|route|bridge|br[uü]cke|supply|supplies|lieferung|fourniture|equipment|ausr[uü]stung|vehicles?|fahrzeug\w*|v[eé]hicule|cleaning|reinigung|nettoyage|maintenance|instandhaltung|entretien|catering|printing|druck|software|hardware|fencing|z[aä]une?|cl[oô]ture|toilet|sanit[aä]r|renovation|sanierung|electric\w*|elektro\w*|heating|heizung|water\s+supply|wasserversorgung|sewage|abwasser|waste|abfall|d[eé]chets|security\s+services|insurance|versicherung|assurance|fuel|kraftstoff|uniform\w*|furniture|m[oö]bel|signage|beschilderung|surveying\s+works|drilling|bohr\w*)\b`)

// --- Austrian public-sector pathway ----------------------------------------
// A Referent / Amtssachverständige / Jurist post in a Land or federal nature
// department is a realistic stepping stone to directing the national park that
// authority co-governs. For such employers we relax the seniority test to
// "meaningful professional role" and instead apply a stricter negative list.

// atOrgRe: Austrian Land / Bund / park administrations.
var atOrgRe = regexp.MustCompile(`(?i)(amt der|landesregierung|land (ober|nieder)?österreich|land (steiermark|kärnten|salzburg|tirol|vorarlberg|burgenland)|land oö|land nö|bundesministerium|\bbml\b|bundesforste|stadt wien|magistrat|umweltbundesamt|nationalpark|naturpark|biosphärenpark|\[land |\[bund\]|\[stadt wien\])`)

// atTopicRe: department / subject signals for nature-related units.
var atTopicRe = regexp.MustCompile(`(?i)(naturschutz|nationalpark|schutzgebiet|natura\s*2000|biodiversit|umweltschutz|umwelt(recht|abteilung|- und)|naturraum|landschaft(sschutz|spflege|splanung)|ökolog|biolog|forst|wald|wildökolog|jagd|fischerei|gewässerökolog|wasserwirtschaft|raumordnung|regionalentwicklung|regionalpolitik|ländlicher raum|agrar|land- und forstwirtschaft|nachhaltigkeit|klimaschutz|artenschutz|moor|alm|weide|abteilung 13|abt\.\s*13|abteilung 8|anlagen- und naturschutz|natur- und geisteswissenschaft|natur/landwirtschaft/technik)`)

// atRoleRe: substantive professional roles (title).
var atRoleRe = regexp.MustCompile(`(?i)(referent|sachverständig|jurist|fachexpert|fachbereich|projektleit|projektmanag|abteilungsleit|gruppenleit|bereichsleit|stabsstellenleit|leitung|leiter|geschäftsführ|direktor|höheren\s+\w*dienst|wissenschaftlichen dienst|akademi|beauftragte|gebietsbetreu|schutzgebietsbetreu|schutzgebietsmanag|nationalparkmanag|natura\s*2000|koordinat|manager|managerin|planstelle|fachkraft mit akademischer|techniker\w* mit akademischer|mit akademischer ausbildung|\bA1\b|\bv1\b|LD\s*1[4-9]|absolvent\w* (der|eines) (universität|studium)|universitätsabsolvent|boku|hochschul)`)

// atNegRe: menial, junior, seasonal, clerical or unrelated titles.
var atNegRe = regexp.MustCompile(`(?i)(reinigung|praktik|ferial|messtechnik|luftmess|laborant|ersatzkraft|karenzvertretung für die verwaltung|lehrling|lehrstelle|saison|ranger|sekretariat|kanzlei|assistenz|assistent|schreibkraft|straßenmeisterei|brückenmeisterei|bauhof|koch\b|köchin|küche|pflege|pädagog|kindergarten|kinderbetreu|ärzt|arzt|amtsärzt|schul|lehrer|kraftfahrer|hausmeister|hauswart|handwerk|facharbeiter|werkmeister|informatik|\bIT\b|it-|software|buchhalt|kassen|lohnverrechn|personalverrechn|controll|sozialarbeit|sozialbetreu|psycholog|therapeut|verkehr|straßenbau|hochbau|tiefbau|brücken|elektro|maschinen|kfz|führerschein|strafrecht|asyl|fremdenwesen|polizei|justiz|militär|bundesheer|zoll|finanz|steuer|gesundheit|krankenanstalt|klinik|veterinär|tierärzt|lebensmittel|marktamt|statistik|dolmetsch|übersetz|redakteur|kommunikation|presse|marketing|tourismus|kultur|museum|archiv|bibliothek|sport|jugend|familie|wohnbau|energie(recht|technik)?\b|strahlenschutz|luftfahrt|geolog|meteorolog|chemiker|labor|vermessung|gis\b|reinigungs|objektbetreu|sicherheitsdienst|portier|servicemitarbeit|gastro)`)

// isAustrianAuthority reports whether the posting comes from a Land / Bund /
// park administration where a professional (non-leadership) role counts.
func isAustrianAuthority(p Posting) bool {
	return p.Region == "austria" && atOrgRe.MatchString(p.Org+" "+p.Snippet+" "+p.Source)
}

// matchAustrianPathway: nature topic + substantive professional role, no junk.
func matchAustrianPathway(p Posting) bool {
	text := p.Title + " \n " + p.Org + " \n " + p.Snippet
	if !atTopicRe.MatchString(text) {
		return false
	}
	if atNegRe.MatchString(p.Title) {
		return false
	}
	return atRoleRe.MatchString(p.Title) || seniorRe.MatchString(p.Title) ||
		// Title is bland but the snippet names a nature unit and a professional grade.
		(atTopicRe.MatchString(p.Snippet) && atRoleRe.MatchString(p.Snippet) && !atNegRe.MatchString(p.Snippet))
}

// Match returns true when a posting plausibly is a senior park / protected-area
// role or consultancy. Cheap, deterministic, tri-lingual. The LLM does the fine ranking.
func Match(p Posting) bool {
	if isAustrianAuthority(p) && matchAustrianPathway(p) {
		return true
	}
	text := p.Title + " \n " + p.Org + " \n " + p.Snippet
	if !topicRe.MatchString(text) {
		return false
	}
	if negRe.MatchString(p.Title) || negRe.MatchString(p.Org) {
		return false
	}
	// Procurement notices: must look like services/consultancy, not works or supplies.
	if p.Tender {
		return !tenderNegRe.MatchString(p.Title) && seniorRe.MatchString(text)
	}
	// Job postings: the seniority/consultancy signal must be in the title itself.
	return seniorRe.MatchString(p.Title)
}

// --- Region heuristics ----------------------------------------------------

var ssaCountries = []string{
	"angola", "benin", "botswana", "burkina", "burundi", "cabo verde", "cape verde", "cameroon", "cameroun", "central african", "centrafric", "chad", "tchad", "comoros", "comores", "congo", "côte d'ivoire", "cote d'ivoire", "ivory coast", "djibouti", "equatorial guinea", "guinée équatoriale", "eritrea", "eswatini", "swaziland", "ethiopia", "éthiopie", "gabon", "gambia", "gambie", "ghana", "guinea", "guinée", "kenya", "lesotho", "liberia", "madagascar", "malawi", "mali", "mauritania", "mauritanie", "mauritius", "maurice", "mozambique", "namibia", "namibie", "niger", "nigeria", "rwanda", "são tomé", "sao tome", "senegal", "sénégal", "seychelles", "sierra leone", "somalia", "somalie", "south africa", "afrique du sud", "südafrika", "south sudan", "soudan du sud", "sudan", "tanzania", "tanzanie", "togo", "uganda", "ouganda", "zambia", "zambie", "zimbabwe", "kinshasa", "nairobi", "lusaka", "harare", "kampala", "dar es salaam", "arusha", "maputo", "windhoek", "gaborone", "lilongwe", "kigali", "addis", "accra", "abidjan", "dakar", "bamako", "libreville", "brazzaville", "bangui", "yaoundé", "yaounde", "douala", "johannesburg", "pretoria", "cape town", "victoria falls", "livingstone", "serengeti", "kruger", "okavango", "kafue", "gorongosa", "garamba", "virunga", "odzala", "zakouma", "pendjari", "niokolo", "afrique", "africa", "afrika", "sub-sahara", "subsahara",
}

var austriaHints = []string{"austria", "österreich", "oesterreich", "autriche", "wien", "vienna", "vienne", "graz", "linz", "salzburg", "innsbruck", "klagenfurt", "villach", "bregenz", "eisenstadt", "st. pölten", "sankt pölten", "hohe tauern", "gesäuse", "gesäuse", "kalkalpen", "donau-auen", "neusiedler", "thayatal", "tirol", "tyrol", "kärnten", "steiermark", "styria", "niederösterreich", "oberösterreich", "burgenland", "vorarlberg", "bundesforste"}

// GuessRegion infers austria | ssa | global from the text, falling back to def.
func GuessRegion(p Posting, def string) string {
	t := strings.ToLower(p.Location + " " + p.Title + " " + p.Org + " " + p.Snippet)
	for _, h := range austriaHints {
		if strings.Contains(t, h) {
			return "austria"
		}
	}
	for _, c := range ssaCountries {
		if strings.Contains(t, c) {
			return "ssa"
		}
	}
	if def == "" || def == "eu" {
		return "global"
	}
	return def
}
