export type InterviewFocus = 'knowledge' | 'project' | 'mixed';
export type InterviewDifficulty = 'easy' | 'medium' | 'hard';
export type MaterialSource = 'upload' | 'paste' | 'url';
export type InterviewAction = 'start' | 'answer' | 'report';
export type ExecutionTraceStatus = 'queued' | 'running' | 'completed' | 'failed';

export interface InterviewExecutionTransition {
  status: ExecutionTraceStatus;
  at: string;
  detail?: string;
  durationMs?: number;
}

export interface InterviewExecutionTrace {
  id: string;
  stage: string;
  label: string;
  detail?: string;
  status: ExecutionTraceStatus;
  agent?: string;
  at: string;
  durationMs?: number;
  transitions?: InterviewExecutionTransition[];
}

export interface InterviewExecutionRun {
  id: string;
  action: InterviewAction;
  title: string;
  status: 'running' | 'completed' | 'failed';
  startedAt: string;
  finishedAt?: string;
  steps: InterviewExecutionTrace[];
}

export interface InterviewMaterial {
  name: string;
  text: string;
  source: MaterialSource;
  mimeType?: string;
  url?: string;
}

export interface InterviewConfig {
  focus: InterviewFocus;
  difficulty: InterviewDifficulty;
  questionCount: number;
  language: 'zh-CN';
  feedbackMode: 'after_each' | 'report_only';
}

export interface EvidenceRef {
  id: string;
  source: 'jd' | 'resume' | 'knowledge' | 'answer';
  label: string;
  excerpt?: string;
  locator?: string;
}

export interface InterviewQuestion {
  id: string;
  index: number;
  text: string;
  kind: 'opening' | 'follow_up' | 'challenge' | 'verification';
  focus: 'knowledge' | 'project';
  topic: string;
  difficulty: InterviewDifficulty;
  depth: number;
  maxDepth: number;
  parentQuestionId?: string;
  evidenceRefs: EvidenceRef[];
  adaptation?: {
    strategy: 'deepen' | 'challenge' | 'verify_resume' | 'cover_jd_gap' | 'switch_topic';
    basedOn: string;
  };
}

export interface InterviewProgress {
  answered: number;
  target: number;
  current: number;
  percent: number;
}

export interface CandidateProfile {
  targetRole?: string;
  seniority?: string;
  topics: string[];
  projects: string[];
}

export interface ClaimCheck {
  claim: string;
  verdict: 'supported' | 'unverified' | 'contradicted' | 'not_in_material';
  evidenceRefs: string[];
}

export interface InterviewFeedback {
  questionId: string;
  deferred?: boolean;
  score: number;
  verdict: 'strong' | 'partial' | 'weak' | 'off_topic' | 'deferred';
  summary: string;
  strengths: string[];
  gaps: string[];
  claimChecks: ClaimCheck[];
  coachTip: string;
  knowledgeVerdict?: 'correct' | 'partial' | 'incorrect' | 'not_applicable' | 'deferred';
  correction?: string;
  evidenceRefs?: EvidenceRef[];
}

export interface InterviewTurn {
  question: InterviewQuestion;
  answer: string;
  feedback: InterviewFeedback;
}

export interface InterviewDimension {
  key: 'knowledge_depth' | 'project_depth' | 'ownership' | 'tradeoffs' | 'communication' | 'jd_fit';
  label: string;
  score: number;
  assessed: boolean;
  sampleCount: number;
  summary: string;
  evidence: string[];
}

export interface InterviewReport {
  interviewId: string;
  state: 'completed';
  overallScore: number;
  readiness: 'ready' | 'borderline' | 'not_ready';
  summary: string;
  dimensions: InterviewDimension[];
  strengths: string[];
  risks: string[];
  jdCoverage: Array<{ requirement: string; status: 'covered' | 'partial' | 'missing'; evidence: string[] }>;
  projectCoverage: Array<{ project: string; depth: number; risks: string[] }>;
  turns: InterviewTurn[];
  nextDrills: string[];
}

export interface StartInterviewRequest {
  action: 'start';
  clientSessionId?: string;
  model?: string;
  config: InterviewConfig;
  materials: { jd?: InterviewMaterial; resume?: InterviewMaterial };
}

export interface StartInterviewResponse {
  interviewId: string;
  state: 'questioning';
  profile: CandidateProfile;
  question: InterviewQuestion;
  progress: InterviewProgress;
}

export interface AnswerInterviewRequest {
  action: 'answer';
  interviewId: string;
  questionId: string;
  clientAnswerId: string;
  answer: { text: string; inputMode: 'text' | 'voice'; durationMs?: number };
}

export interface AnswerInterviewResponse {
  interviewId: string;
  state: 'questioning' | 'completed';
  feedback: InterviewFeedback;
  nextQuestion: InterviewQuestion | null;
  progress: InterviewProgress;
  reportReady: boolean;
}

export interface InterviewSnapshot {
  interviewId: string;
  state: 'questioning' | 'completed';
  profile: CandidateProfile;
  currentQuestion: InterviewQuestion | null;
  turns: InterviewTurn[];
  progress: InterviewProgress;
  reportReady: boolean;
}

export interface InterviewSessionEvent {
  eventId: string;
  sequence: number;
  commandId?: string;
  type: 'answer.started' | 'answer.committed' | 'answer.failed' | string;
  createdAt: string;
}

export interface InterviewSessionEventPage {
  interviewId: string;
  events: InterviewSessionEvent[];
  nextSequence: number;
}

export interface InterviewReviewReference {
  evidenceId: string;
  title: string;
  answer: string;
  locator?: string;
}

export interface InterviewReviewTurn extends InterviewTurn {
  inputMode: 'text' | 'voice';
  durationMs?: number;
  references: InterviewReviewReference[];
  answeredAt: string;
}

export interface InterviewReviewSnapshot {
  schemaVersion: '1.0.0';
  interviewId: string;
  state: 'questioning' | 'completed';
  startedAt: string;
  generatedAt: string;
  turns: InterviewReviewTurn[];
}

export interface ReportInterviewRequest {
  action: 'report';
  interviewId: string;
}

export interface InterviewApiError {
  error: { code: string; message: string; retryable?: boolean; field?: string } | string;
}
