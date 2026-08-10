package interview

import (
	"context"
	"encoding/json"
	"time"
)

type Action string

const (
	ActionStart  Action = "start"
	ActionAnswer Action = "answer"
	ActionReport Action = "report"
)

type InterviewState string

const (
	StateAwaitingAnswer InterviewState = "awaiting_answer"
	StateCompleted      InterviewState = "completed"
)

type Focus string

const (
	FocusMixed     Focus = "mixed"
	FocusKnowledge Focus = "knowledge"
	FocusProjects  Focus = "projects"
)

type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyMedium Difficulty = "medium"
	DifficultyHard   Difficulty = "hard"
)

type FeedbackMode string

const (
	FeedbackImmediate FeedbackMode = "immediate"
	FeedbackDeferred  FeedbackMode = "deferred"
)

type InputMode string

const (
	InputModeText  InputMode = "text"
	InputModeVoice InputMode = "voice"
)

type SourceKind string

const (
	SourceJD        SourceKind = "jd"
	SourceResume    SourceKind = "resume"
	SourceKnowledge SourceKind = "knowledge"
)

type QuestionKind string

const (
	QuestionKnowledge    QuestionKind = "knowledge"
	QuestionProject      QuestionKind = "project"
	QuestionBehavioral   QuestionKind = "behavioral"
	QuestionFollowUp     QuestionKind = "follow_up"
	QuestionPrerequisite QuestionKind = "prerequisite"
)

type PolicyAction string

const (
	PolicyInitial      PolicyAction = "initial"
	PolicyPrerequisite PolicyAction = "prerequisite"
	PolicyFollowUp     PolicyAction = "follow_up"
	PolicyAdvance      PolicyAction = "advance"
	PolicyComplete     PolicyAction = "complete"
)

type ClaimVerdict string

const (
	ClaimSupported     ClaimVerdict = "supported"
	ClaimUnverified    ClaimVerdict = "unverified"
	ClaimContradicted  ClaimVerdict = "contradicted"
	ClaimNotInMaterial ClaimVerdict = "not_in_material"
)

// MaterialInput accepts either a plain JSON string or an object with text and
// name. Supporting both keeps file-upload adapters out of the domain service.
type MaterialInput struct {
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
}

func (m *MaterialInput) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		m.Text = text
		return nil
	}
	type materialAlias MaterialInput
	var value materialAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = MaterialInput(value)
	return nil
}

type MaterialsInput struct {
	JD     *MaterialInput `json:"jd,omitempty"`
	Resume *MaterialInput `json:"resume,omitempty"`
}

type InterviewConfig struct {
	Focus         Focus        `json:"focus"`
	Difficulty    Difficulty   `json:"difficulty"`
	QuestionCount int          `json:"questionCount"`
	Language      string       `json:"language"`
	FeedbackMode  FeedbackMode `json:"feedbackMode"`
}

type StartRequest struct {
	Action          Action          `json:"action"`
	ClientSessionID string          `json:"clientSessionId"`
	Model           string          `json:"model,omitempty"`
	Config          InterviewConfig `json:"config"`
	Materials       MaterialsInput  `json:"materials"`
}

type StartResponse struct {
	InterviewID string         `json:"interviewId"`
	State       InterviewState `json:"state"`
	Profile     Profile        `json:"profile"`
	Question    Question       `json:"question"`
	Progress    Progress       `json:"progress"`
}

type AnswerPayload struct {
	Text       string    `json:"text"`
	InputMode  InputMode `json:"inputMode"`
	DurationMS int64     `json:"durationMs,omitempty"`
}

type AnswerRequest struct {
	Action      Action        `json:"action"`
	InterviewID string        `json:"interviewId"`
	QuestionID  string        `json:"questionId"`
	Answer      AnswerPayload `json:"answer"`
}

type AnswerFeedback struct {
	Assessment Assessment `json:"assessment"`
	Summary    string     `json:"summary"`
}

type AnswerResponse struct {
	InterviewID  string         `json:"interviewId"`
	State        InterviewState `json:"state"`
	Feedback     AnswerFeedback `json:"feedback"`
	NextQuestion *Question      `json:"nextQuestion,omitempty"`
	Progress     Progress       `json:"progress"`
	ReportReady  bool           `json:"reportReady"`
}

type ReportRequest struct {
	Action      Action `json:"action"`
	InterviewID string `json:"interviewId"`
}

type ReportResponse struct {
	InterviewID string         `json:"interviewId"`
	State       InterviewState `json:"state"`
	Report      Report         `json:"report"`
}

type EvidenceRef struct {
	SourceID string     `json:"sourceId"`
	Kind     SourceKind `json:"kind"`
	AnchorID string     `json:"anchorId"`
	Locator  string     `json:"locator"`
	Quote    string     `json:"quote"`
}

type SourceAnchor struct {
	ID       string     `json:"id"`
	SourceID string     `json:"sourceId"`
	Kind     SourceKind `json:"kind"`
	Locator  string     `json:"locator"`
	Text     string     `json:"text"`
}

type SourceDocument struct {
	ID      string     `json:"id"`
	Kind    SourceKind `json:"kind"`
	Name    string     `json:"name,omitempty"`
	Content string     `json:"-"`
}

type SourceIndex struct {
	Documents map[string]SourceDocument `json:"documents"`
	Anchors   map[string]SourceAnchor   `json:"anchors"`
	Order     []string                  `json:"order"`
}

type ProfilePoint struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type JDProfile struct {
	Title            string         `json:"title,omitempty"`
	Requirements     []ProfilePoint `json:"requirements"`
	Responsibilities []ProfilePoint `json:"responsibilities"`
}

type ResumeProfile struct {
	Headline string         `json:"headline,omitempty"`
	Skills   []string       `json:"skills"`
	Projects []ProfilePoint `json:"projects"`
}

type CoveragePoint struct {
	ID           string        `json:"id"`
	Area         Focus         `json:"area"`
	Label        string        `json:"label"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

// CoverageCandidate is the bounded choice set offered to CoveragePlanner.
// Priority is assigned by deterministic product policy (higher wins), while
// the planner balances it against actual question coverage and semantic gaps.
type CoverageCandidate struct {
	CoveragePointID string        `json:"coveragePointId"`
	Area            Focus         `json:"area"`
	Label           string        `json:"label"`
	Priority        int           `json:"priority"`
	QuestionCount   int           `json:"questionCount"`
	LastAskedTurn   int           `json:"lastAskedTurn"` // 1-based; zero means never asked.
	EvidenceRefs    []EvidenceRef `json:"evidenceRefs"`
}

type Profile struct {
	JD       JDProfile       `json:"jd"`
	Resume   ResumeProfile   `json:"resume"`
	Coverage []CoveragePoint `json:"coverage"`
}

type QuestionAdaptation struct {
	Trigger           PolicyAction `json:"trigger"`
	Reason            string       `json:"reason"`
	BasedOnQuestionID string       `json:"basedOnQuestionId,omitempty"`
	FollowUpAxis      string       `json:"followUpAxis,omitempty"`
	Depth             int          `json:"depth"`
}

type Question struct {
	ID              string             `json:"id"`
	RootID          string             `json:"rootId"`
	Text            string             `json:"text"`
	Kind            QuestionKind       `json:"kind"`
	Difficulty      Difficulty         `json:"difficulty"`
	CoveragePointID string             `json:"coveragePointId"`
	EvidenceRefs    []EvidenceRef      `json:"evidenceRefs"`
	Adaptation      QuestionAdaptation `json:"adaptation"`
}

type Assessment struct {
	Correctness   int           `json:"correctness"`
	Depth         int           `json:"depth"`
	Specificity   int           `json:"specificity"`
	Ownership     int           `json:"ownership"`
	Metrics       int           `json:"metrics"`
	Tradeoffs     int           `json:"tradeoffs"`
	FactualErrors []string      `json:"factualErrors"`
	Strengths     []string      `json:"strengths"`
	Gaps          []string      `json:"gaps"`
	EvidenceRefs  []EvidenceRef `json:"evidenceRefs"`
	ClaimChecks   []ClaimCheck  `json:"claimChecks"`
}

type ClaimCheck struct {
	Claim        string        `json:"claim"`
	Verdict      ClaimVerdict  `json:"verdict"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type PolicyDecision struct {
	Action          PolicyAction `json:"action"`
	Reason          string       `json:"reason"`
	FollowUpAxis    string       `json:"followUpAxis,omitempty"`
	Difficulty      Difficulty   `json:"difficulty"`
	CoveragePointID string       `json:"coveragePointId"`
	RootID          string       `json:"rootId"`
	FollowUpDepth   int          `json:"followUpDepth"`
}

type Progress struct {
	Answered      int `json:"answered"`
	Total         int `json:"total"`
	Current       int `json:"current"`
	FollowUpDepth int `json:"followUpDepth"`
}

type AnswerRecord struct {
	Question   Question       `json:"question"`
	Answer     AnswerPayload  `json:"answer"`
	Assessment Assessment     `json:"assessment"`
	Decision   PolicyDecision `json:"decision"`
	AnsweredAt time.Time      `json:"answeredAt"`
}

type AuditTurn struct {
	QuestionID      string        `json:"questionId"`
	RootID          string        `json:"rootId"`
	CoveragePointID string        `json:"coveragePointId"`
	EvidenceRefs    []EvidenceRef `json:"evidenceRefs"`
	InputMode       InputMode     `json:"inputMode"`
	DurationMS      int64         `json:"durationMs,omitempty"`
	AnsweredAt      time.Time     `json:"answeredAt"`
}

type ReportAudit struct {
	ClientSessionID string      `json:"clientSessionId"`
	StartedAt       time.Time   `json:"startedAt"`
	CompletedAt     time.Time   `json:"completedAt"`
	GeneratedAt     time.Time   `json:"generatedAt"`
	Turns           []AuditTurn `json:"turns"`
}

type Report struct {
	OverallScore int            `json:"overallScore"`
	Summary      string         `json:"summary"`
	Strengths    []string       `json:"strengths"`
	Gaps         []string       `json:"gaps"`
	EvidenceRefs []EvidenceRef  `json:"evidenceRefs"`
	Profile      Profile        `json:"profile"`
	Turns        []AnswerRecord `json:"turns"`
	Audit        ReportAudit    `json:"audit"`
}

// InterviewSession is the aggregate persisted by Store. Callers should treat
// loaded values as snapshots and save them with optimistic version checking.
type InterviewSession struct {
	ID              string          `json:"id"`
	ClientSessionID string          `json:"clientSessionId"`
	Model           string          `json:"model,omitempty"`
	Config          InterviewConfig `json:"config"`
	State           InterviewState  `json:"state"`
	Profile         Profile         `json:"profile"`
	Sources         SourceIndex     `json:"sources"`
	CurrentQuestion *Question       `json:"currentQuestion,omitempty"`
	Answers         []AnswerRecord  `json:"answers"`
	CoverageCursor  int             `json:"coverageCursor"`
	Report          *Report         `json:"report,omitempty"`
	StartedAt       time.Time       `json:"startedAt"`
	CompletedAt     time.Time       `json:"completedAt,omitempty"`
	Version         int64           `json:"version"`
}

type QuestionDraft struct {
	Text         string        `json:"text"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type ReportDraft struct {
	Summary      string        `json:"summary"`
	Strengths    []string      `json:"strengths"`
	Gaps         []string      `json:"gaps"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

// PlanCoverageRequest is passed only when the deterministic policy has
// decided to advance. It gives the planner semantic signals and bounded IDs;
// it does not authorize the model to create or mutate coverage points.
type PlanCoverageRequest struct {
	Config                 InterviewConfig      `json:"config"`
	CurrentCoveragePointID string               `json:"currentCoveragePointId"`
	PreviousQuestion       Question             `json:"previousQuestion"`
	PreviousAssessment     Assessment           `json:"previousAssessment"`
	Candidates             []CoverageCandidate  `json:"candidates"`
	QuestionKindCounts     map[QuestionKind]int `json:"questionKindCounts"`
	History                []AnswerRecord       `json:"history"`
	RemainingQuestions     int                  `json:"remainingQuestions"`
}

// CoverageSelection is a proposal. The service must still verify the ID
// against its current session snapshot before committing the transition.
type CoverageSelection struct {
	CoveragePointID string   `json:"coveragePointId"`
	Reason          string   `json:"reason"`
	Signals         []string `json:"signals"`
}

type RepairInstruction struct {
	Reason          string        `json:"reason"`
	AllowedEvidence []EvidenceRef `json:"allowedEvidence"`
}

type GenerateQuestionRequest struct {
	Profile  Profile            `json:"profile"`
	Decision PolicyDecision     `json:"decision"`
	Anchors  []SourceAnchor     `json:"anchors"`
	History  []AnswerRecord     `json:"history"`
	Repair   *RepairInstruction `json:"repair,omitempty"`
}

type AssessAnswerRequest struct {
	Question Question           `json:"question"`
	Answer   AnswerPayload      `json:"answer"`
	Anchors  []SourceAnchor     `json:"anchors"`
	History  []AnswerRecord     `json:"history"`
	Repair   *RepairInstruction `json:"repair,omitempty"`
}

type GenerateReportRequest struct {
	Profile Profile            `json:"profile"`
	Answers []AnswerRecord     `json:"answers"`
	Anchors []SourceAnchor     `json:"anchors"`
	Repair  *RepairInstruction `json:"repair,omitempty"`
}

type Agent interface {
	GenerateQuestion(context.Context, GenerateQuestionRequest) (QuestionDraft, error)
	AssessAnswer(context.Context, AssessAnswerRequest) (Assessment, error)
	GenerateReport(context.Context, GenerateReportRequest) (ReportDraft, error)
}

// CoveragePlanner is optional and intentionally separate from Agent so an
// existing question/assessment/report implementation remains source-compatible.
type CoveragePlanner interface {
	PlanCoverage(context.Context, PlanCoverageRequest) (CoverageSelection, error)
}

type KnowledgeQuery struct {
	Model  string `json:"model,omitempty"`
	Focus  Focus  `json:"focus"`
	JD     string `json:"jd,omitempty"`
	Resume string `json:"resume,omitempty"`
}

type KnowledgeDocument struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type KnowledgeRetriever interface {
	Retrieve(context.Context, KnowledgeQuery) ([]KnowledgeDocument, error)
}

type Store interface {
	Create(context.Context, InterviewSession) error
	Load(context.Context, string) (InterviewSession, error)
	Save(context.Context, InterviewSession, int64) error
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type Dependencies struct {
	Agent     Agent
	Planner   CoveragePlanner
	Retriever KnowledgeRetriever
	Store     Store
	Clock     Clock
	IDs       IDGenerator
}
