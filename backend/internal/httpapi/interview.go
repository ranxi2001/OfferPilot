package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"offerpilot/backend/internal/executiontrace"
	"offerpilot/backend/internal/interview"
)

type webEvidenceRef struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Label   string `json:"label"`
	Excerpt string `json:"excerpt,omitempty"`
	Locator string `json:"locator,omitempty"`
}

type webQuestion struct {
	ID               string           `json:"id"`
	Index            int              `json:"index"`
	Text             string           `json:"text"`
	Kind             string           `json:"kind"`
	Focus            string           `json:"focus"`
	Topic            string           `json:"topic"`
	Difficulty       string           `json:"difficulty"`
	Depth            int              `json:"depth"`
	MaxDepth         int              `json:"maxDepth"`
	ParentQuestionID string           `json:"parentQuestionId,omitempty"`
	EvidenceRefs     []webEvidenceRef `json:"evidenceRefs"`
	Adaptation       *webAdaptation   `json:"adaptation,omitempty"`
}

type webAdaptation struct {
	Strategy string `json:"strategy"`
	BasedOn  string `json:"basedOn"`
}

type webProgress struct {
	Answered int `json:"answered"`
	Target   int `json:"target"`
	Current  int `json:"current"`
	Percent  int `json:"percent"`
}

type webCandidateProfile struct {
	TargetRole string   `json:"targetRole,omitempty"`
	Seniority  string   `json:"seniority,omitempty"`
	Topics     []string `json:"topics"`
	Projects   []string `json:"projects"`
}

type webClaimCheck struct {
	Claim        string   `json:"claim"`
	Verdict      string   `json:"verdict"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type webFeedback struct {
	QuestionID       string           `json:"questionId"`
	Score            int              `json:"score"`
	Verdict          string           `json:"verdict"`
	Summary          string           `json:"summary"`
	Strengths        []string         `json:"strengths"`
	Gaps             []string         `json:"gaps"`
	ClaimChecks      []webClaimCheck  `json:"claimChecks"`
	CoachTip         string           `json:"coachTip"`
	KnowledgeVerdict string           `json:"knowledgeVerdict,omitempty"`
	Correction       string           `json:"correction,omitempty"`
	EvidenceRefs     []webEvidenceRef `json:"evidenceRefs,omitempty"`
}

type webTurn struct {
	Question webQuestion `json:"question"`
	Answer   string      `json:"answer"`
	Feedback webFeedback `json:"feedback"`
}

type webDimension struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Score       int      `json:"score"`
	Assessed    bool     `json:"assessed"`
	SampleCount int      `json:"sampleCount"`
	Summary     string   `json:"summary"`
	Evidence    []string `json:"evidence"`
}

type webJDCoverage struct {
	Requirement string   `json:"requirement"`
	Status      string   `json:"status"`
	Evidence    []string `json:"evidence"`
}

type webProjectCoverage struct {
	Project string   `json:"project"`
	Depth   int      `json:"depth"`
	Risks   []string `json:"risks"`
}

type webReport struct {
	InterviewID     string               `json:"interviewId"`
	State           string               `json:"state"`
	OverallScore    int                  `json:"overallScore"`
	Readiness       string               `json:"readiness"`
	Summary         string               `json:"summary"`
	Dimensions      []webDimension       `json:"dimensions"`
	Strengths       []string             `json:"strengths"`
	Risks           []string             `json:"risks"`
	JDCoverage      []webJDCoverage      `json:"jdCoverage"`
	ProjectCoverage []webProjectCoverage `json:"projectCoverage"`
	Turns           []webTurn            `json:"turns"`
	NextDrills      []string             `json:"nextDrills"`
}

type interviewStreamEnvelope struct {
	Type   string                `json:"type"`
	Trace  *executiontrace.Event `json:"trace,omitempty"`
	Status int                   `json:"status,omitempty"`
	Data   json.RawMessage       `json:"data,omitempty"`
}

type capturedInterviewResponse struct {
	status int
	body   []byte
}

// handleInterviewStream reuses the compatibility handler so the streamed and
// non-streamed routes cannot drift in request validation or response mapping.
func (s *Server) handleInterviewStream(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Accel-Buffering", "no")
	response.WriteHeader(http.StatusOK)
	flusher, _ := response.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	events := make(chan executiontrace.Event, 64)
	result := make(chan capturedInterviewResponse, 1)
	ctx := executiontrace.WithSink(request.Context(), func(event executiontrace.Event) {
		select {
		case events <- event:
		case <-request.Context().Done():
		}
	})
	tracedRequest := request.WithContext(ctx)

	go func() {
		capture := newBufferedResponseWriter()
		var handlerErr error
		span := executiontrace.Start(ctx, "request", "Execute interview action", "")
		defer func() {
			if recovered := recover(); recovered != nil {
				handlerErr = errors.New("interview handler panic")
				capture.Reset()
				writeAPIError(capture, http.StatusInternalServerError, "internal", "Internal server error", true, "")
			}
			if capture.Status() >= http.StatusBadRequest && handlerErr == nil {
				handlerErr = errors.New("interview request failed")
			}
			span.End(handlerErr, "")
			result <- capturedInterviewResponse{status: capture.Status(), body: append([]byte(nil), capture.Body()...)}
			close(events)
		}()
		s.handleInterview(capture, tracedRequest)
	}()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				captured := <-result
				payload := bytes.TrimSpace(captured.body)
				if !json.Valid(payload) {
					payload = []byte(`{"error":{"code":"internal","message":"Interview service returned an invalid response","retryable":true}}`)
					captured.status = http.StatusInternalServerError
				}
				_ = writeNDJSON(response, flusher, interviewStreamEnvelope{
					Type: "result", Status: captured.status, Data: json.RawMessage(payload),
				})
				return
			}
			if err := writeNDJSON(response, flusher, interviewStreamEnvelope{Type: "trace", Trace: &event}); err != nil {
				return
			}
		case <-request.Context().Done():
			return
		}
	}
}

func writeNDJSON(response io.Writer, flusher http.Flusher, payload interviewStreamEnvelope) error {
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

type bufferedResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header)}
}

func (w *bufferedResponseWriter) Header() http.Header { return w.header }

func (w *bufferedResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *bufferedResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *bufferedResponseWriter) Body() []byte { return w.body.Bytes() }

func (w *bufferedResponseWriter) Reset() {
	w.header = make(http.Header)
	w.body.Reset()
	w.status = 0
}

func (s *Server) handleInterview(response http.ResponseWriter, request *http.Request) {
	if !s.config.ModelConfigured {
		writeAPIError(response, http.StatusServiceUnavailable, string(interview.CodeUnavailable), "Interview model is not configured", true, "")
		return
	}
	data, err := readBody(response, request, s.config.MaxInterviewBytes)
	if err != nil {
		writeReadError(response, err)
		return
	}
	var envelope struct {
		Action interview.Action `json:"action"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_json", "Invalid JSON", false, "")
		return
	}

	switch envelope.Action {
	case interview.ActionStart:
		var input interview.StartRequest
		if err := json.Unmarshal(data, &input); err != nil {
			writeAPIError(response, http.StatusBadRequest, "invalid_json", "Invalid start request", false, "")
			return
		}
		normalizeStartRequest(&input)
		if strings.TrimSpace(input.ClientSessionID) == "" {
			created, createErr := s.sessions.Create()
			if createErr != nil {
				writeAPIError(response, http.StatusInternalServerError, "session_create_failed", "Could not create interview session", true, "")
				return
			}
			input.ClientSessionID = created.ID
		}
		output, serviceErr := s.interview.Start(request.Context(), input)
		if serviceErr != nil {
			writeInterviewError(response, serviceErr)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"interviewId": output.InterviewID,
			"state":       webState(output.State),
			"profile":     mapProfile(output.Profile),
			"question":    mapQuestion(output.Question, output.Progress.Current, &output.Profile),
			"progress":    mapProgress(output.Progress),
		})

	case interview.ActionAnswer:
		var input interview.AnswerRequest
		if err := json.Unmarshal(data, &input); err != nil {
			writeAPIError(response, http.StatusBadRequest, "invalid_json", "Invalid answer request", false, "")
			return
		}
		output, serviceErr := s.interview.Answer(request.Context(), input)
		if serviceErr != nil {
			writeInterviewError(response, serviceErr)
			return
		}
		var next *webQuestion
		if output.NextQuestion != nil {
			mapped := mapQuestion(*output.NextQuestion, output.Progress.Current, nil)
			next = &mapped
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"interviewId":  output.InterviewID,
			"state":        webState(output.State),
			"feedback":     mapFeedback(input.QuestionID, output.Feedback.Assessment, output.Feedback.Summary, questionFocusFromEvidence(output.Feedback.Assessment.EvidenceRefs)),
			"nextQuestion": next,
			"progress":     mapProgress(output.Progress),
			"reportReady":  output.ReportReady,
		})

	case interview.ActionReport:
		var input interview.ReportRequest
		if err := json.Unmarshal(data, &input); err != nil {
			writeAPIError(response, http.StatusBadRequest, "invalid_json", "Invalid report request", false, "")
			return
		}
		output, serviceErr := s.interview.Report(request.Context(), input)
		if serviceErr != nil {
			writeInterviewError(response, serviceErr)
			return
		}
		writeJSON(response, http.StatusOK, mapReport(output.InterviewID, output.Report))

	default:
		writeAPIError(response, http.StatusBadRequest, "validation", "action must be start, answer, or report", false, "action")
	}
}

func normalizeStartRequest(input *interview.StartRequest) {
	if input.Config.Focus == "project" {
		input.Config.Focus = interview.FocusProjects
	}
	switch input.Config.FeedbackMode {
	case "after_each":
		input.Config.FeedbackMode = interview.FeedbackImmediate
	case "report_only":
		input.Config.FeedbackMode = interview.FeedbackDeferred
	}
}

func mapProfile(profile interview.Profile) webCandidateProfile {
	topics := make([]string, 0)
	for _, point := range profile.JD.Requirements {
		topics = appendUniqueString(topics, point.Label)
	}
	for _, point := range profile.JD.Responsibilities {
		topics = appendUniqueString(topics, point.Label)
	}
	for _, skill := range profile.Resume.Skills {
		topics = appendUniqueString(topics, skill)
	}
	projects := make([]string, 0, len(profile.Resume.Projects))
	for _, project := range profile.Resume.Projects {
		projects = appendUniqueString(projects, project.Label)
	}
	return webCandidateProfile{
		TargetRole: profile.JD.Title,
		Topics:     topics,
		Projects:   projects,
	}
}

func mapQuestion(question interview.Question, index int, profile *interview.Profile) webQuestion {
	focus := questionFocus(question)
	topic := question.CoveragePointID
	if profile != nil {
		for _, point := range profile.Coverage {
			if point.ID == question.CoveragePointID {
				topic = point.Label
				break
			}
		}
	}
	if (topic == "" || topic == question.CoveragePointID) && len(question.EvidenceRefs) > 0 {
		topic = question.EvidenceRefs[0].Quote
	}
	topic = truncateRunes(topic, 90)

	kind := "opening"
	switch question.Adaptation.Trigger {
	case interview.PolicyFollowUp:
		kind = "follow_up"
	case interview.PolicyPrerequisite:
		kind = "verification"
	case interview.PolicyAdvance:
		kind = "opening"
	}

	var adaptation *webAdaptation
	if question.Adaptation.Trigger != interview.PolicyInitial {
		strategy := "switch_topic"
		switch question.Adaptation.Trigger {
		case interview.PolicyFollowUp:
			strategy = "deepen"
			if focus == "project" && question.Adaptation.FollowUpAxis == "ownership" {
				strategy = "verify_resume"
			}
		case interview.PolicyPrerequisite:
			strategy = "challenge"
		case interview.PolicyAdvance:
			strategy = "switch_topic"
		}
		adaptation = &webAdaptation{Strategy: strategy, BasedOn: adaptationReason(question.Adaptation)}
	}
	if index <= 0 {
		index = 1
	}
	return webQuestion{
		ID:               question.ID,
		Index:            index,
		Text:             question.Text,
		Kind:             kind,
		Focus:            focus,
		Topic:            topic,
		Difficulty:       string(question.Difficulty),
		Depth:            question.Adaptation.Depth,
		MaxDepth:         2,
		ParentQuestionID: question.Adaptation.BasedOnQuestionID,
		EvidenceRefs:     mapQuestionEvidenceRefs(question.EvidenceRefs),
		Adaptation:       adaptation,
	}
}

func questionFocus(question interview.Question) string {
	if question.Kind == interview.QuestionProject {
		return "project"
	}
	for _, ref := range question.EvidenceRefs {
		if ref.Kind == interview.SourceResume {
			return "project"
		}
	}
	return "knowledge"
}

func questionFocusFromEvidence(refs []interview.EvidenceRef) string {
	for _, ref := range refs {
		if ref.Kind == interview.SourceResume {
			return "project"
		}
	}
	return "knowledge"
}

func mapEvidenceRefs(refs []interview.EvidenceRef) []webEvidenceRef {
	result := make([]webEvidenceRef, 0, len(refs))
	for _, ref := range refs {
		result = append(result, webEvidenceRef{
			ID:      ref.AnchorID,
			Source:  string(ref.Kind),
			Label:   evidenceLabel(ref),
			Excerpt: ref.Quote,
			Locator: ref.Locator,
		})
	}
	return result
}

func mapQuestionEvidenceRefs(refs []interview.EvidenceRef) []webEvidenceRef {
	result := mapEvidenceRefs(refs)
	for index, ref := range refs {
		if ref.Kind == interview.SourceKnowledge {
			result[index].Excerpt = knowledgeQuestionExcerpt(ref.Quote)
		}
	}
	return result
}

func knowledgeQuestionExcerpt(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"问题：", "问题:"} {
			if strings.HasPrefix(line, prefix) {
				return truncateRunes(line, 180)
			}
		}
	}
	for _, marker := range []string{"参考内容：", "参考内容:"} {
		if position := strings.Index(value, marker); position > 0 {
			return truncateRunes(value[:position], 180)
		}
	}
	return truncateRunes(value, 180)
}

func evidenceLabel(ref interview.EvidenceRef) string {
	label := map[interview.SourceKind]string{
		interview.SourceJD:        "JD 要求",
		interview.SourceResume:    "简历项目",
		interview.SourceKnowledge: "知识库",
	}[ref.Kind]
	if label == "" {
		label = string(ref.Kind)
	}
	return label + " · " + ref.SourceID
}

func mapProgress(progress interview.Progress) webProgress {
	percent := 0
	if progress.Total > 0 {
		percent = int(math.Round(float64(progress.Answered) / float64(progress.Total) * 100))
	}
	return webProgress{
		Answered: progress.Answered,
		Target:   progress.Total,
		Current:  progress.Current,
		Percent:  minInt(percent, 100),
	}
}

func mapFeedback(questionID string, assessment interview.Assessment, summary, focus string) webFeedback {
	score := assessmentScoreForFocus(assessment, focus)
	verdict := "weak"
	switch {
	case score >= 80:
		verdict = "strong"
	case score >= 55:
		verdict = "partial"
	}
	checks := make([]webClaimCheck, 0, len(assessment.ClaimChecks))
	for _, check := range assessment.ClaimChecks {
		ids := make([]string, 0, len(check.EvidenceRefs))
		for _, ref := range check.EvidenceRefs {
			ids = append(ids, ref.AnchorID)
		}
		checks = append(checks, webClaimCheck{Claim: check.Claim, Verdict: string(check.Verdict), EvidenceRefs: ids})
	}
	coachTip := "用结论、证据、个人动作、指标口径和方案取舍重新组织回答。"
	if len(assessment.Gaps) > 0 {
		coachTip = assessment.Gaps[0]
	}
	feedback := webFeedback{
		QuestionID:   questionID,
		Score:        score,
		Verdict:      verdict,
		Summary:      strings.TrimSpace(summary),
		Strengths:    nonNilStrings(assessment.Strengths),
		Gaps:         nonNilStrings(assessment.Gaps),
		ClaimChecks:  checks,
		CoachTip:     coachTip,
		EvidenceRefs: mapEvidenceRefs(assessment.EvidenceRefs),
	}
	if focus == "knowledge" {
		switch {
		case assessment.Correctness >= 4:
			feedback.KnowledgeVerdict = "correct"
		case assessment.Correctness >= 2:
			feedback.KnowledgeVerdict = "partial"
		default:
			feedback.KnowledgeVerdict = "incorrect"
		}
	} else {
		feedback.KnowledgeVerdict = "not_applicable"
	}
	if len(assessment.FactualErrors) > 0 {
		feedback.Correction = strings.Join(assessment.FactualErrors, "；")
	}
	if feedback.Summary == "" {
		feedback.Summary = feedbackSummary(assessment)
	}
	return feedback
}

func mapReport(interviewID string, report interview.Report) webReport {
	turns := make([]webTurn, 0, len(report.Turns))
	for index, record := range report.Turns {
		turns = append(turns, webTurn{
			Question: mapQuestion(record.Question, index+1, &report.Profile),
			Answer:   record.Answer.Text,
			Feedback: mapFeedback(record.Question.ID, record.Assessment, feedbackSummary(record.Assessment), questionFocus(record.Question)),
		})
	}
	dimensions := reportDimensions(report.Turns)
	jdCoverage := reportJDCoverage(report.Profile, report.Turns)
	projectCoverage := reportProjectCoverage(report.Profile, report.Turns)
	readiness := reportReadiness(report.OverallScore, report.Profile, report.Turns, jdCoverage, projectCoverage)
	nextDrills := nonNilStrings(report.Gaps)
	if len(nextDrills) == 0 {
		nextDrills = []string{"针对本轮证据重答一遍，并补足个人职责、测量口径与取舍。"}
	}
	return webReport{
		InterviewID:     interviewID,
		State:           "completed",
		OverallScore:    report.OverallScore,
		Readiness:       readiness,
		Summary:         report.Summary,
		Dimensions:      dimensions,
		Strengths:       nonNilStrings(report.Strengths),
		Risks:           nonNilStrings(report.Gaps),
		JDCoverage:      jdCoverage,
		ProjectCoverage: projectCoverage,
		Turns:           turns,
		NextDrills:      nextDrills,
	}
}

func reportDimensions(turns []interview.AnswerRecord) []webDimension {
	type dimensionSpec struct {
		key, label string
		value      func(interview.Assessment) float64
	}
	specs := []dimensionSpec{
		{key: "knowledge_depth", label: "知识深度", value: func(a interview.Assessment) float64 { return float64(a.Correctness+a.Depth) / 2 }},
		{key: "project_depth", label: "项目深度", value: func(a interview.Assessment) float64 { return float64(a.Depth+a.Specificity) / 2 }},
		{key: "ownership", label: "个人贡献", value: func(a interview.Assessment) float64 { return float64(a.Ownership) }},
		{key: "tradeoffs", label: "方案取舍", value: func(a interview.Assessment) float64 { return float64(a.Tradeoffs) }},
		{key: "communication", label: "表达清晰度", value: func(a interview.Assessment) float64 { return float64(a.Specificity) }},
		{key: "jd_fit", label: "岗位匹配", value: func(a interview.Assessment) float64 { return float64(a.Correctness+a.Specificity) / 2 }},
	}
	result := make([]webDimension, 0, len(specs))
	for _, spec := range specs {
		var total float64
		count := 0
		for _, turn := range turns {
			if !dimensionIncludesTurn(spec.key, turn.Question) {
				continue
			}
			total += spec.value(turn.Assessment)
			count++
		}
		score := 0
		summary := "本轮没有覆盖该能力，未评估，不计为 0 分。"
		if count > 0 {
			score = int(math.Round(total / float64(count) * 20))
			summary = fmt.Sprintf("基于 %d 轮对应题型的结构化评估聚合，得分 %d/100。", count, score)
		}
		result = append(result, webDimension{
			Key: spec.key, Label: spec.label, Score: score, Assessed: count > 0, SampleCount: count,
			Summary: summary, Evidence: dimensionEvidence(turns, spec.key),
		})
	}
	return result
}

func dimensionEvidence(turns []interview.AnswerRecord, key string) []string {
	result := make([]string, 0, 3)
	for _, turn := range turns {
		if !dimensionIncludesTurn(key, turn.Question) {
			continue
		}
		items := turn.Assessment.Strengths
		if key == "jd_fit" || key == "project_depth" {
			items = append(append([]string{}, items...), turn.Assessment.Gaps...)
		}
		for _, item := range items {
			result = appendUniqueString(result, item)
			if len(result) == 3 {
				return result
			}
		}
	}
	return result
}

func dimensionIncludesTurn(key string, question interview.Question) bool {
	switch key {
	case "knowledge_depth":
		return questionFocus(question) == "knowledge"
	case "project_depth", "ownership":
		return questionFocus(question) == "project"
	case "jd_fit":
		return questionHasEvidenceKind(question, interview.SourceJD)
	default:
		return true
	}
}

func questionHasEvidenceKind(question interview.Question, kind interview.SourceKind) bool {
	for _, ref := range question.EvidenceRefs {
		if ref.Kind == kind {
			return true
		}
	}
	return false
}

func adaptationReason(adaptation interview.QuestionAdaptation) string {
	switch adaptation.Trigger {
	case interview.PolicyPrerequisite:
		return "上一轮存在事实或前置理解问题，先核对基础原理。"
	case interview.PolicyFollowUp:
		switch adaptation.FollowUpAxis {
		case "ownership":
			return "上一轮没有讲清个人职责边界，继续核对本人贡献。"
		case "metrics":
			return "上一轮缺少指标、基线或测量口径，继续追问可验证数据。"
		case "tradeoff":
			return "上一轮缺少备选方案与代价分析，继续追问技术取舍。"
		case "verification":
			return "上一轮回答与简历材料存在冲突，继续核对陈述和证据边界。"
		case "principle":
			return "上一轮没有讲清核心机制，继续追问原理与因果链。"
		case "boundary":
			return "上一轮缺少适用前提或失效边界，继续追问约束条件。"
		case "example":
			return "上一轮缺少具体场景，继续追问输入、过程、输出与验证方式。"
		default:
			return "上一轮回答仍不够具体，继续追问可核验细节。"
		}
	case interview.PolicyAdvance:
		if strings.TrimSpace(adaptation.Reason) != "" {
			return adaptation.Reason
		}
		return "当前覆盖点已完成，切换到尚未覆盖的岗位或项目能力。"
	default:
		return adaptation.Reason
	}
}

func reportJDCoverage(profile interview.Profile, turns []interview.AnswerRecord) []webJDCoverage {
	points := append(append([]interview.ProfilePoint{}, profile.JD.Requirements...), profile.JD.Responsibilities...)
	points = mergeProfilePoints(points)
	result := make([]webJDCoverage, 0, len(points))
	for _, point := range points {
		records := recordsForEvidence(turns, point.EvidenceRefs)
		status := "missing"
		if len(records) > 0 {
			status = "partial"
			if averageRecordScore(records) >= 70 && recordsHaveNoKnowledgeError(records) {
				status = "covered"
			}
		}
		result = append(result, webJDCoverage{Requirement: point.Label, Status: status, Evidence: recordIDs(records)})
	}
	return result
}

func mergeProfilePoints(points []interview.ProfilePoint) []interview.ProfilePoint {
	result := make([]interview.ProfilePoint, 0, len(points))
	positions := make(map[string]int, len(points))
	for _, point := range points {
		key := strings.ToLower(strings.Join(strings.Fields(point.Label), " "))
		if key == "" {
			continue
		}
		if position, exists := positions[key]; exists {
			merged := result[position].EvidenceRefs
			for _, ref := range point.EvidenceRefs {
				found := false
				for _, existing := range merged {
					if existing.AnchorID == ref.AnchorID {
						found = true
						break
					}
				}
				if !found {
					merged = append(merged, ref)
				}
			}
			result[position].EvidenceRefs = merged
			continue
		}
		positions[key] = len(result)
		result = append(result, point)
	}
	return result
}

func reportReadiness(score int, profile interview.Profile, turns []interview.AnswerRecord, jd []webJDCoverage, projects []webProjectCoverage) string {
	if score < 55 || len(turns) == 0 {
		return "not_ready"
	}

	hasJD := len(profile.JD.Requirements)+len(profile.JD.Responsibilities) > 0
	hasProjects := len(profile.Resume.Projects) > 0
	coverageComplete := true
	if hasJD {
		coverageComplete = false
		for _, item := range jd {
			if item.Status != "missing" {
				coverageComplete = true
				break
			}
		}
	}
	if hasProjects {
		projectCovered := false
		for _, item := range projects {
			if item.Depth > 0 {
				projectCovered = true
				break
			}
		}
		coverageComplete = coverageComplete && projectCovered
	}
	if !hasJD && !hasProjects {
		coverageComplete = false
		for _, turn := range turns {
			if questionFocus(turn.Question) == "knowledge" {
				coverageComplete = true
				break
			}
		}
	}

	if score >= 75 && coverageComplete && !hasCriticalInterviewRisk(turns) {
		return "ready"
	}
	return "borderline"
}

func hasCriticalInterviewRisk(turns []interview.AnswerRecord) bool {
	for _, turn := range turns {
		if len(turn.Assessment.FactualErrors) > 0 {
			return true
		}
		for _, check := range turn.Assessment.ClaimChecks {
			if check.Verdict == interview.ClaimContradicted {
				return true
			}
		}
	}
	return false
}

func reportProjectCoverage(profile interview.Profile, turns []interview.AnswerRecord) []webProjectCoverage {
	result := make([]webProjectCoverage, 0, len(profile.Resume.Projects))
	for _, project := range profile.Resume.Projects {
		records := recordsForEvidence(turns, project.EvidenceRefs)
		risks := make([]string, 0)
		for _, record := range records {
			for _, gap := range record.Assessment.Gaps {
				risks = appendUniqueString(risks, gap)
			}
		}
		result = append(result, webProjectCoverage{Project: project.Label, Depth: len(records), Risks: risks})
	}
	return result
}

func recordsForEvidence(turns []interview.AnswerRecord, refs []interview.EvidenceRef) []interview.AnswerRecord {
	wanted := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		wanted[ref.AnchorID] = struct{}{}
	}
	result := make([]interview.AnswerRecord, 0)
	for _, turn := range turns {
		for _, ref := range turn.Question.EvidenceRefs {
			if _, exists := wanted[ref.AnchorID]; exists {
				result = append(result, turn)
				break
			}
		}
	}
	return result
}

func recordIDs(records []interview.AnswerRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Question.ID)
	}
	return result
}

func averageRecordScore(records []interview.AnswerRecord) int {
	if len(records) == 0 {
		return 0
	}
	total := 0
	for _, record := range records {
		total += assessmentScoreForFocus(record.Assessment, questionFocus(record.Question))
	}
	return int(math.Round(float64(total) / float64(len(records))))
}

func assessmentScore(assessment interview.Assessment) int {
	total := assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Ownership + assessment.Metrics + assessment.Tradeoffs
	return int(math.Round(float64(total) / 30 * 100))
}

func assessmentScoreForFocus(assessment interview.Assessment, focus string) int {
	if focus == "knowledge" {
		total := assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Tradeoffs
		return int(math.Round(float64(total) / 20 * 100))
	}
	return assessmentScore(assessment)
}

func recordsHaveNoKnowledgeError(records []interview.AnswerRecord) bool {
	for _, record := range records {
		if record.Assessment.Correctness <= 2 || len(record.Assessment.FactualErrors) > 0 {
			return false
		}
	}
	return true
}

func feedbackSummary(assessment interview.Assessment) string {
	if len(assessment.FactualErrors) > 0 {
		return "本轮存在需要纠正的事实或前置理解。"
	}
	if len(assessment.Gaps) > 0 {
		return assessment.Gaps[0]
	}
	if len(assessment.Strengths) > 0 {
		return assessment.Strengths[0]
	}
	return "本轮评估已完成。"
}

func writeInterviewError(response http.ResponseWriter, err error) {
	var domainError *interview.DomainError
	if !errors.As(err, &domainError) {
		writeAPIError(response, http.StatusInternalServerError, "internal", "Interview service failed", true, "")
		return
	}
	status := http.StatusInternalServerError
	retryable := false
	switch domainError.Code {
	case interview.CodeValidation:
		status = http.StatusBadRequest
	case interview.CodeNotFound:
		status = http.StatusNotFound
	case interview.CodeConflict, interview.CodeInvalidState:
		status = http.StatusConflict
		retryable = domainError.Code == interview.CodeConflict
	case interview.CodeGrounding:
		status = http.StatusUnprocessableEntity
	case interview.CodeUnavailable:
		status = http.StatusServiceUnavailable
		retryable = true
	case interview.CodeInternal:
		retryable = true
	}
	writeAPIError(response, status, string(domainError.Code), domainError.Message, retryable, domainError.Field)
}

func webState(state interview.InterviewState) string {
	if state == interview.StateCompleted {
		return "completed"
	}
	return "questioning"
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
