'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Activity,
  AlertCircle,
  ArrowRight,
  BarChart3,
  BrainCircuit,
  BriefcaseBusiness,
  Check,
  Crosshair,
  Database,
  Download,
  FileText,
  Gauge,
  Loader2,
  Mic,
  Play,
  RotateCcw,
  Send,
  ShieldCheck,
  Square,
  Target,
  Volume2,
  X,
} from 'lucide-react';
import { ExecutionTimeline } from '@/components/ExecutionTimeline';
import { MaterialInput } from '@/components/MaterialInput';
import { AnswerSubmissionGuard, reusableAnswerSubmission } from '@/lib/answer-submission';
import {
  clearClientAnswerDescriptor,
  getOrCreateClientAnswerId,
  readClientAnswerDescriptor,
  type ClientAnswerDescriptor,
  type ClientAnswerStorage,
} from '@/lib/client-answer-id';
import {
  clearExecutionHistory,
  readExecutionHistory,
  settleLatestRunningExecution,
  writeExecutionHistory,
} from '@/lib/execution-history';
import { InterviewRequestError, interviewClient } from '@/lib/interview-client';
import {
  buildInterviewReviewHtml,
  downloadInterviewReview,
  recordingToDataUrl,
  type ReviewRecording,
} from '@/lib/interview-review-export';
import {
  clearInterviewEventSequence,
  deriveRecoveredInterviewStage,
  hasUnresolvedAnswerEvent,
  latestAnswerEventOutcome,
  mergeInterviewEvents,
  readInterviewEventSequence,
  recoveryEventAfter,
  writeInterviewEventSequence,
} from '@/lib/interview-recovery';
import { failExecutionInRuns, mergeTraceIntoRuns } from '@/lib/execution-trace';
import {
  createBrowserQuestionSpeechController,
  type QuestionSpeechController,
  type QuestionSpeechPhase,
} from '@/lib/question-speech';
import {
  VoiceAnswerRecordingCache,
  VoiceTranscriptionError,
  voiceTranscriptionErrorMessage,
} from '@/lib/voice-answer-recording';
import type {
  AnswerInterviewRequest,
  CandidateProfile,
  InterviewAction,
  InterviewConfig,
  InterviewDifficulty,
  InterviewExecutionRun,
  InterviewExecutionTrace,
  InterviewFeedback,
  InterviewFocus,
  InterviewMaterial,
  InterviewProgress,
  InterviewQuestion,
  InterviewReport,
  InterviewTurn,
} from '@/types/interview';

type Phase = 'setup' | 'questioning' | 'feedback' | 'report';
type FailedOperation = InterviewAction | null;

interface ReleaseRecordingOptions {
  clearAnswerTimer?: boolean;
  clearSamples?: boolean;
  updateState?: boolean;
}

const DEFAULT_PROGRESS: InterviewProgress = { answered: 0, target: 7, current: 1, percent: 0 };
const RECOVERY_POLL_ATTEMPTS = 300;
const RECOVERY_POLL_DELAY_MS = 1000;

const focusOptions: Array<{ value: InterviewFocus; label: string; icon: typeof BrainCircuit }> = [
  { value: 'mixed', label: '综合拷打', icon: Crosshair },
  { value: 'knowledge', label: '知识硬核', icon: BrainCircuit },
  { value: 'project', label: '项目深挖', icon: BriefcaseBusiness },
];

const difficultyOptions: Array<{ value: InterviewDifficulty; label: string }> = [
  { value: 'easy', label: '基础' },
  { value: 'medium', label: '进阶' },
  { value: 'hard', label: '压力' },
];

export function InterviewView() {
  const [phase, setPhase] = useState<Phase>('setup');
  const [jd, setJd] = useState<InterviewMaterial | null>(null);
  const [resume, setResume] = useState<InterviewMaterial | null>(null);
  const [config, setConfig] = useState<InterviewConfig>({
    focus: 'mixed',
    difficulty: 'hard',
    questionCount: 7,
    language: 'zh-CN',
    feedbackMode: 'after_each',
  });
  const [interviewId, setInterviewId] = useState<string | null>(null);
  const [profile, setProfile] = useState<CandidateProfile | null>(null);
  const [question, setQuestion] = useState<InterviewQuestion | null>(null);
  const [pendingQuestion, setPendingQuestion] = useState<InterviewQuestion | null>(null);
  const [progress, setProgress] = useState<InterviewProgress>(DEFAULT_PROGRESS);
  const [answer, setAnswer] = useState('');
  const [feedback, setFeedback] = useState<InterviewFeedback | null>(null);
  const [turns, setTurns] = useState<InterviewTurn[]>([]);
  const [report, setReport] = useState<InterviewReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [busyLabel, setBusyLabel] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [speechPhase, setSpeechPhase] = useState<QuestionSpeechPhase>('idle');
  const [isRecording, setIsRecording] = useState(false);
  const [canRetryTranscription, setCanRetryTranscription] = useState(false);
  const [executionRuns, setExecutionRuns] = useState<InterviewExecutionRun[]>([]);
  const [failedOperation, setFailedOperation] = useState<FailedOperation>(null);
  const [exportingReview, setExportingReview] = useState(false);

  const answerStartedAt = useRef(0);
  const audioContextRef = useRef<AudioContext | null>(null);
  const sourceRef = useRef<MediaStreamAudioSourceNode | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const samplesRef = useRef<Float32Array[]>([]);
  const sampleRateRef = useRef(16000);
  const recordingGenerationRef = useRef(0);
  const transcriptionAttemptRef = useRef(0);
  const voiceRecordingCacheRef = useRef(new VoiceAnswerRecordingCache());
  const answerSubmissionGuardRef = useRef(new AnswerSubmissionGuard());
  const mountedRef = useRef(false);
  const executionSequenceRef = useRef(0);
  const clientAnswerRef = useRef<ClientAnswerDescriptor | null>(null);
  const pendingAnswerRef = useRef<AnswerInterviewRequest | null>(null);
  const questionSpeechRef = useRef<QuestionSpeechController | null>(null);
  const reviewRecordingsRef = useRef(new Map<string, ReviewRecording>());

  function answerStorage(): ClientAnswerStorage | null {
    try {
      return window.sessionStorage;
    } catch {
      return null;
    }
  }

  function answerIdFor(activeInterviewId: string, activeQuestionId: string): string {
    const current = clientAnswerRef.current;
    if (current?.interviewId === activeInterviewId && current.questionId === activeQuestionId) {
      return current.clientAnswerId;
    }

    const clientAnswerId = getOrCreateClientAnswerId(answerStorage(), activeInterviewId, activeQuestionId);
    clientAnswerRef.current = {
      interviewId: activeInterviewId,
      questionId: activeQuestionId,
      clientAnswerId,
    };
    return clientAnswerId;
  }

  const releaseRecordingResources = useCallback((options: ReleaseRecordingOptions = {}) => {
    const {
      clearAnswerTimer = true,
      clearSamples = true,
      updateState = true,
    } = options;

    recordingGenerationRef.current += 1;

    const processor = processorRef.current;
    processorRef.current = null;
    if (processor) {
      processor.onaudioprocess = null;
      try {
        processor.disconnect();
      } catch {}
    }

    const source = sourceRef.current;
    sourceRef.current = null;
    if (source) {
      try {
        source.disconnect();
      } catch {}
    }

    const stream = streamRef.current;
    streamRef.current = null;
    stream?.getTracks().forEach((track) => track.stop());

    const audioContext = audioContextRef.current;
    audioContextRef.current = null;
    if (audioContext && audioContext.state !== 'closed') {
      void audioContext.close().catch(() => {});
    }

    if (clearSamples) samplesRef.current = [];
    if (clearAnswerTimer) answerStartedAt.current = 0;
    if (updateState && mountedRef.current) setIsRecording(false);
  }, []);

  const clearCachedVoiceRecording = useCallback((updateState = true) => {
    transcriptionAttemptRef.current += 1;
    voiceRecordingCacheRef.current.clear();
    if (updateState && mountedRef.current) setCanRetryTranscription(false);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    const questionSpeech = createBrowserQuestionSpeechController(({ phase: nextPhase }) => {
      if (mountedRef.current) setSpeechPhase(nextPhase);
    });
    questionSpeechRef.current = questionSpeech;
    return () => {
      mountedRef.current = false;
      questionSpeechRef.current = null;
      questionSpeech.dispose();
      answerSubmissionGuardRef.current.invalidate();
      clearCachedVoiceRecording(false);
      releaseRecordingResources({ updateState: false });
    };
  }, [clearCachedVoiceRecording, releaseRecordingResources]);

  useEffect(() => {
    const storage = answerStorage();
    const descriptor = readClientAnswerDescriptor(storage);
    if (!descriptor) return;

    const controller = new AbortController();
    const restore = async () => {
      setBusy(true);
      setBusyLabel('正在恢复上次面试');
      setError(null);
      try {
        let sequence = readInterviewEventSequence(storage, descriptor.interviewId);
        let events = [] as Awaited<ReturnType<typeof interviewClient.events>>['events'];
        let eventsAvailable = true;

        try {
          const page = await interviewClient.events(
            descriptor.interviewId,
            recoveryEventAfter(sequence),
            100,
            controller.signal,
          );
          events = mergeInterviewEvents(events, page.events);
          sequence = Math.max(sequence, page.nextSequence);
          writeInterviewEventSequence(storage, descriptor.interviewId, sequence);
        } catch (eventError) {
          if ((eventError as Error).name === 'AbortError') throw eventError;
          eventsAvailable = false;
        }
        let snapshot = await interviewClient.snapshot(descriptor.interviewId, controller.signal);

        for (let attempt = 0; eventsAvailable && hasUnresolvedAnswerEvent(events) && attempt < RECOVERY_POLL_ATTEMPTS; attempt += 1) {
          await waitForRecoveryPoll(RECOVERY_POLL_DELAY_MS, controller.signal);
          let page;
          try {
            page = await interviewClient.events(descriptor.interviewId, sequence, 100, controller.signal);
          } catch (eventError) {
            if ((eventError as Error).name === 'AbortError') throw eventError;
            eventsAvailable = false;
            break;
          }
          events = mergeInterviewEvents(events, page.events);
          sequence = Math.max(sequence, page.nextSequence);
          writeInterviewEventSequence(storage, descriptor.interviewId, sequence);
          snapshot = await interviewClient.snapshot(descriptor.interviewId, controller.signal);
        }

        if (snapshot.interviewId !== descriptor.interviewId) {
          throw new InterviewRequestError('恢复的面试标识不匹配。', { code: 'invalid_response', status: 502 });
        }
        const recovered = deriveRecoveredInterviewStage(snapshot, descriptor.questionId);
        if (!recovered) {
          throw new InterviewRequestError('上次面试没有可恢复的题目。', { code: 'invalid_response', status: 502 });
        }
        if (controller.signal.aborted || !mountedRef.current) return;

        clientAnswerRef.current = descriptor;
        pendingAnswerRef.current = null;
        setInterviewId(snapshot.interviewId);
        setProfile(snapshot.profile);
        setQuestion(recovered.question);
        setPendingQuestion(recovered.pendingQuestion);
        setFeedback(recovered.feedback);
        setTurns(snapshot.turns);
        setProgress(snapshot.progress);
        setReport(null);
        setAnswer('');
        setFailedOperation(null);
        const storedRuns = readExecutionHistory(storage, snapshot.interviewId);
        const answerCommitted = snapshot.turns.some((turn) => turn.question.id === descriptor.questionId);
        const recoveredOutcome = answerCommitted
          ? 'completed'
          : latestAnswerEventOutcome(events) === 'failed' ? 'failed' : null;
        setExecutionRuns(recoveredOutcome
          ? settleLatestRunningExecution(storedRuns, 'answer', recoveredOutcome, new Date().toISOString())
          : storedRuns);
        setPhase(recovered.phase);
        if (recovered.phase === 'questioning') {
          answerIdFor(snapshot.interviewId, recovered.question.id);
          answerStartedAt.current = Date.now();
        }
        if (eventsAvailable && hasUnresolvedAnswerEvent(events)) {
          setError('后台回答仍在处理中，请稍后刷新以同步最新题目。');
        }
      } catch (restoreError) {
        if (controller.signal.aborted || !mountedRef.current) return;
        const requestError = restoreError instanceof InterviewRequestError ? restoreError : null;
        if (requestError && !requestError.retryable && requestError.status < 500) {
          clearExecutionHistory(storage, descriptor.interviewId);
          clearInterviewEventSequence(storage, descriptor.interviewId);
          clearClientAnswerDescriptor(storage);
          clientAnswerRef.current = null;
        }
        const canRetryRecovery = !requestError || requestError.retryable || requestError.status >= 500;
        setError(canRetryRecovery
          ? '暂时无法恢复上次面试，请稍后刷新；你也可以直接开始新面试。'
          : '上次面试已失效，请重新开始。');
      } finally {
        if (!controller.signal.aborted && mountedRef.current) {
          setBusy(false);
          setBusyLabel('');
        }
      }
    };

    void restore();
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!interviewId) return;
    writeExecutionHistory(answerStorage(), interviewId, executionRuns);
  }, [executionRuns, interviewId]);

  function beginExecution(action: InterviewAction, title: string) {
    const id = `execution-${Date.now()}-${++executionSequenceRef.current}`;
    const startedAt = new Date().toISOString();
    const acceptedStep: InterviewExecutionTrace = {
      id: `${id}:accepted`,
      stage: 'request',
      label: action === 'answer' ? '回答已接收' : action === 'start' ? '面试请求已接收' : '报告请求已接收',
      detail: '请求已进入 OfferPilot，等待 Go Harness 执行。',
      status: 'completed',
      at: startedAt,
    };
    setExecutionRuns((current) => [...current, {
      id,
      action,
      title,
      status: 'running',
      startedAt,
      steps: [acceptedStep],
    }]);
    return id;
  }

  function recordExecutionTrace(runId: string, trace: InterviewExecutionTrace) {
    if (!mountedRef.current) return;
    setExecutionRuns((current) => mergeTraceIntoRuns(current, runId, trace));
    if (trace.status === 'queued' || trace.status === 'running') {
      setBusyLabel(trace.detail || trace.label);
    }
  }

  function completeExecution(runId: string) {
    const finishedAt = new Date().toISOString();
    setExecutionRuns((current) => current.map((run) => (
      run.id === runId ? { ...run, status: 'completed', finishedAt } : run
    )));
  }

  function failExecution(runId: string, message: string) {
    const finishedAt = new Date().toISOString();
    setExecutionRuns((current) => failExecutionInRuns(current, runId, message, finishedAt));
  }

  function handleExecutionError(errorValue: unknown, operation: InterviewAction, runId: string) {
    const message = interviewErrorMessage(errorValue);
    failExecution(runId, message);
    setError(message);
    const retryable = !(errorValue instanceof InterviewRequestError) || errorValue.retryable;
    setFailedOperation(retryable ? operation : null);
  }

  function retryFailedOperation() {
    if (busy) return;
    if (failedOperation === 'start') void startInterview();
    if (failedOperation === 'answer') {
      const pending = pendingAnswerRef.current;
      if (pending) {
        void submitAnswer(
          pending.answer.text,
          pending.answer.inputMode,
          pending.answer.durationMs,
        );
      } else {
        void submitAnswer(answer);
      }
    }
    if (failedOperation === 'report') void loadReport();
  }

  async function startInterview() {
    if (!jd && !resume) {
      setError('至少提供 JD 或简历；项目深挖需要简历材料。');
      return;
    }
    if (config.focus === 'project' && !resume) {
      setError('项目深挖模式需要先上传或粘贴简历。');
      return;
    }
    answerSubmissionGuardRef.current.invalidate();
    clearCachedVoiceRecording();
    const previousInterviewId = readClientAnswerDescriptor(answerStorage())?.interviewId;

    setBusy(true);
    setBusyLabel('正在建立岗位画像与证据索引');
    setError(null);
    setFailedOperation(null);
    pendingAnswerRef.current = null;
    const runId = beginExecution('start', '建立面试上下文');
    try {
      const data = await interviewClient.start({
        action: 'start',
        config,
        materials: { jd: jd ?? undefined, resume: resume ?? undefined },
      }, (trace) => recordExecutionTrace(runId, trace));
      completeExecution(runId);
      if (previousInterviewId && previousInterviewId !== data.interviewId) {
        clearExecutionHistory(answerStorage(), previousInterviewId);
        clearInterviewEventSequence(answerStorage(), previousInterviewId);
      }
      setInterviewId(data.interviewId);
      setProfile(data.profile);
      setQuestion(data.question);
      answerIdFor(data.interviewId, data.question.id);
      setProgress(data.progress);
      setTurns([]);
      setFeedback(null);
      setPendingQuestion(null);
      setPhase('questioning');
      answerStartedAt.current = Date.now();
      speakQuestion(data.question.text);
    } catch (err) {
      handleExecutionError(err, 'start', runId);
    } finally {
      setBusy(false);
      setBusyLabel('');
    }
  }

  async function submitAnswer(
    text: string,
    inputMode: 'text' | 'voice' = 'text',
    durationMs = Math.max(0, Date.now() - answerStartedAt.current),
  ) {
    if (!interviewId || !question || !text.trim() || busy) return;
    const submitted = text.trim();
    const reusable = reusableAnswerSubmission(
      pendingAnswerRef.current,
      interviewId,
      question.id,
      submitted,
    );
    if (pendingAnswerRef.current?.questionId === question.id && !reusable) {
      setError('上次提交结果尚未确认，请使用“重试本步”按原回答重试。');
      setFailedOperation('answer');
      return;
    }
    stopQuestionSpeech();
    setBusy(true);
    setBusyLabel('正在检索证据、核对事实并规划追问');
    setError(null);
    setCanRetryTranscription(false);
    setFailedOperation(null);
    const request: AnswerInterviewRequest = reusable ?? {
      action: 'answer',
      interviewId,
      questionId: question.id,
      clientAnswerId: answerIdFor(interviewId, question.id),
      answer: {
        text: submitted,
        inputMode,
        durationMs,
      },
    };
    pendingAnswerRef.current = request;
    const submissionGeneration = answerSubmissionGuardRef.current.begin();
    const isCurrentSubmission = () => mountedRef.current
      && answerSubmissionGuardRef.current.isCurrent(submissionGeneration);
    const runId = beginExecution('answer', `第 ${question.index} 轮回答评估`);
    try {
      const data = await interviewClient.answer(request, (trace) => {
        if (isCurrentSubmission()) recordExecutionTrace(runId, trace);
      });
      if (!isCurrentSubmission()) return;
      completeExecution(runId);
      pendingAnswerRef.current = null;
      const submittedRecording = inputMode === 'voice'
        ? voiceRecordingCacheRef.current.currentFor(interviewId, question.id)
        : null;
      if (submittedRecording) {
        try {
          reviewRecordingsRef.current.set(question.id, await recordingToDataUrl(
            question.id,
            submittedRecording.blob,
            submittedRecording.durationMs,
          ));
        } catch {
          // Textual review remains available if this browser cannot encode audio.
        }
      }
      clearCachedVoiceRecording();
      releaseRecordingResources();
      setFeedback(data.feedback);
      setPendingQuestion(data.nextQuestion);
      setProgress(data.progress);
      setTurns((current) => [...current, { question, answer: submitted, feedback: data.feedback }]);
      setAnswer('');
      setPhase('feedback');
    } catch (err) {
      if (isCurrentSubmission()) {
        setAnswer(submitted);
        handleExecutionError(err, 'answer', runId);
      }
    } finally {
      if (isCurrentSubmission()) {
        setBusy(false);
        setBusyLabel('');
      }
    }
  }

  function continueInterview() {
    if (!pendingQuestion) {
      void loadReport();
      return;
    }
    clearCachedVoiceRecording();
    const nextQuestion = pendingQuestion;
    pendingAnswerRef.current = null;
    setQuestion(nextQuestion);
    setPendingQuestion(null);
    setFeedback(null);
    setPhase('questioning');
    if (interviewId) answerIdFor(interviewId, nextQuestion.id);
    answerStartedAt.current = Date.now();
    speakQuestion(nextQuestion.text);
  }

  async function loadReport() {
    if (!interviewId || busy) return;
    clearCachedVoiceRecording();
    releaseRecordingResources();
    stopQuestionSpeech();
    setBusy(true);
    setBusyLabel('正在生成证据化面试报告');
    setError(null);
    setFailedOperation(null);
    const runId = beginExecution('report', '生成面试报告');
    try {
      const data = await interviewClient.report(
        { action: 'report', interviewId },
        (trace) => recordExecutionTrace(runId, trace),
      );
      completeExecution(runId);
      setReport(data);
      setPhase('report');
    } catch (err) {
      handleExecutionError(err, 'report', runId);
    } finally {
      setBusy(false);
      setBusyLabel('');
    }
  }

  function resetInterview() {
    answerSubmissionGuardRef.current.invalidate();
    clearCachedVoiceRecording();
    releaseRecordingResources();
    stopQuestionSpeech();
    if (interviewId) {
      clearExecutionHistory(answerStorage(), interviewId);
      clearInterviewEventSequence(answerStorage(), interviewId);
    }
    clientAnswerRef.current = null;
    pendingAnswerRef.current = null;
    clearClientAnswerDescriptor(answerStorage());
    setPhase('setup');
    setInterviewId(null);
    setProfile(null);
    setQuestion(null);
    setPendingQuestion(null);
    setProgress(DEFAULT_PROGRESS);
    setAnswer('');
    setFeedback(null);
    setTurns([]);
    setReport(null);
    setError(null);
    setBusy(false);
    setBusyLabel('');
    setExecutionRuns([]);
    setFailedOperation(null);
    reviewRecordingsRef.current.clear();
  }

  async function exportReview() {
    if (!interviewId || exportingReview) return;
    setExportingReview(true);
    setError(null);
    try {
      const review = await interviewClient.review(interviewId);
      downloadInterviewReview(buildInterviewReviewHtml({
        review,
        recordings: [...reviewRecordingsRef.current.values()],
        executionRuns,
      }), interviewId);
    } catch (exportError) {
      setError(exportError instanceof InterviewRequestError
        ? `复盘导出失败：${exportError.message}`
        : '复盘导出失败，请稍后重试。');
    } finally {
      setExportingReview(false);
    }
  }

  function speakQuestion(text: string) {
    void questionSpeechRef.current?.speak(text);
  }

  function stopQuestionSpeech() {
    questionSpeechRef.current?.cancel();
  }

  function toggleQuestionSpeech(text: string) {
    const controller = questionSpeechRef.current;
    if (!controller) return;
    if (controller.active) {
      controller.cancel();
      return;
    }
    void controller.speak(text);
  }

  async function startRecording() {
    stopQuestionSpeech();
    const generation = recordingGenerationRef.current + 1;
    recordingGenerationRef.current = generation;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (!mountedRef.current || recordingGenerationRef.current !== generation) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      streamRef.current = stream;
      const AudioContextCtor = window.AudioContext || (window as typeof window & { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
      const audioContext = new AudioContextCtor();
      audioContextRef.current = audioContext;
      const source = audioContext.createMediaStreamSource(stream);
      sourceRef.current = source;
      const processor = audioContext.createScriptProcessor(4096, 1, 1);
      processorRef.current = processor;
      samplesRef.current = [];
      sampleRateRef.current = audioContext.sampleRate;
      processor.onaudioprocess = (event) => {
        samplesRef.current.push(new Float32Array(event.inputBuffer.getChannelData(0)));
      };
      source.connect(processor);
      processor.connect(audioContext.destination);
      clearCachedVoiceRecording();
      setIsRecording(true);
      setError(null);
    } catch (err) {
      if (recordingGenerationRef.current === generation) {
        releaseRecordingResources({ clearAnswerTimer: false });
        if (mountedRef.current) setError(`无法开始录音：${(err as Error).message}`);
      }
    }
  }

  async function stopRecording() {
    const chunks = samplesRef.current;
    const sourceRate = sampleRateRef.current;
    const durationMs = Math.max(0, Date.now() - answerStartedAt.current);
    const activeInterviewId = interviewId;
    const activeQuestionId = question?.id;
    const wav = encodeWav(chunks, sourceRate);
    releaseRecordingResources({ clearAnswerTimer: false });
    if (!activeInterviewId || !activeQuestionId) return;

    voiceRecordingCacheRef.current.cache(wav, durationMs, activeInterviewId, activeQuestionId);
    setCanRetryTranscription(false);
    await analyzeCachedVoiceRecording();
  }

  async function analyzeCachedVoiceRecording() {
    const activeInterviewId = interviewId;
    const activeQuestionId = question?.id;
    if (!activeInterviewId || !activeQuestionId) return;
    const recording = voiceRecordingCacheRef.current.currentFor(activeInterviewId, activeQuestionId);
    if (!recording) {
      setCanRetryTranscription(false);
      setError('录音缓存已失效，请重新录音。');
      return;
    }

    const attempt = transcriptionAttemptRef.current + 1;
    transcriptionAttemptRef.current = attempt;
    setBusy(true);
    setBusyLabel('正在转写回答');
    setError(null);
    setFailedOperation(null);
    setCanRetryTranscription(false);
    try {
      const result = await voiceRecordingCacheRef.current.transcribe(activeInterviewId, activeQuestionId);
      if (!result
        || !mountedRef.current
        || transcriptionAttemptRef.current !== attempt
        || !voiceRecordingCacheRef.current.isCurrent(recording)) return;
      setBusy(false);
      setBusyLabel('');
      await submitAnswer(result.text, 'voice', result.recording.durationMs);
    } catch (err) {
      if (!mountedRef.current
        || transcriptionAttemptRef.current !== attempt
        || !voiceRecordingCacheRef.current.isCurrent(recording)) return;
      const reason = voiceTranscriptionErrorMessage(err);
      const retryable = err instanceof VoiceTranscriptionError && err.retryable;
      setError(retryable
        ? `语音转写失败：${reason}。录音已保留，无需重新回答。`
        : `语音转写失败：${reason}`);
      setCanRetryTranscription(retryable);
    } finally {
      if (mountedRef.current
        && transcriptionAttemptRef.current === attempt
        && voiceRecordingCacheRef.current.isCurrent(recording)) {
        setBusy(false);
        setBusyLabel('');
      }
    }
  }

  if (phase === 'setup') {
    return (
      <div className="flex-1 overflow-y-auto bg-slate-50/70 px-4 py-5 sm:px-6">
        <div className="mx-auto max-w-6xl space-y-5">
          <header className="flex flex-col gap-3 border-b border-slate-200 pb-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase text-slate-400">
                <Activity size={13} className="text-emerald-500" />
                Adaptive interview harness
              </div>
              <h2 className="text-xl font-bold text-primary">面试作战台</h2>
            </div>
            <div className="flex items-center gap-2 text-xs text-slate-500">
              <ShieldCheck size={14} className="text-emerald-500" />
              证据锚定
              <span className="text-slate-300">/</span>
              自适应追问
              <span className="text-slate-300">/</span>
              会话记忆
            </div>
          </header>

          <div className="grid gap-4 lg:grid-cols-2">
            <MaterialInput
              label="职位描述（JD）"
              description="岗位要求、职责与加分项"
              emptyName="粘贴的职位描述"
              value={jd}
              onChange={setJd}
            />
            <MaterialInput
              label="候选人简历"
              description="项目经历、个人贡献与量化结果"
              emptyName="粘贴的简历"
              value={resume}
              onChange={setResume}
            />
          </div>

          <section className="rounded-lg border border-slate-200 bg-white shadow-card">
            <div className="grid gap-5 p-4 lg:grid-cols-[1.25fr_1fr_auto] lg:items-end">
              <ControlGroup label="拷打重点" icon={Target}>
                <div className="grid grid-cols-3 gap-1 rounded-md bg-slate-100 p-1">
                  {focusOptions.map(({ value, label, icon: Icon }) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => setConfig((current) => ({ ...current, focus: value }))}
                      className={`flex h-9 min-w-0 items-center justify-center gap-1.5 rounded px-2 text-xs font-medium ${
                        config.focus === value ? 'bg-white text-primary shadow-sm' : 'text-slate-500 hover:text-slate-700'
                      }`}
                    >
                      <Icon size={13} className="shrink-0" />
                      <span className="truncate">{label}</span>
                    </button>
                  ))}
                </div>
              </ControlGroup>

              <ControlGroup label="面试强度" icon={Gauge}>
                <div className="grid grid-cols-3 gap-1 rounded-md bg-slate-100 p-1">
                  {difficultyOptions.map(({ value, label }) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => setConfig((current) => ({ ...current, difficulty: value }))}
                      className={`h-9 rounded px-3 text-xs font-medium ${
                        config.difficulty === value ? 'bg-white text-primary shadow-sm' : 'text-slate-500 hover:text-slate-700'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </ControlGroup>

              <ControlGroup label="目标轮数" icon={BarChart3}>
                <div className="flex h-10 items-center rounded-md border border-slate-200 bg-white p-1">
                  {[5, 7, 9].map((count) => (
                    <button
                      key={count}
                      type="button"
                      onClick={() => setConfig((current) => ({ ...current, questionCount: count }))}
                      className={`h-8 w-10 rounded text-xs font-semibold ${
                        config.questionCount === count ? 'bg-primary text-white' : 'text-slate-500 hover:bg-slate-100'
                      }`}
                    >
                      {count}
                    </button>
                  ))}
                </div>
              </ControlGroup>
            </div>

            <div className="flex flex-col gap-3 border-t border-slate-100 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="text-xs text-slate-400">
                {jd ? `JD ${jd.text.length} 字` : '未加载 JD'}
                <span className="mx-2 text-slate-300">/</span>
                {resume ? `简历 ${resume.text.length} 字` : '未加载简历'}
              </div>
              <button
                type="button"
                onClick={() => void startInterview()}
                disabled={busy || (!jd && !resume)}
                className="flex h-11 min-w-36 items-center justify-center gap-2 rounded-md bg-primary px-5 text-sm font-semibold text-white shadow-sm hover:bg-primary-light disabled:cursor-not-allowed disabled:opacity-40"
              >
                {busy ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
                开始拷打
              </button>
            </div>
          </section>

          <StatusMessage
            error={error}
            busy={busy}
            label={busyLabel}
            onRetry={failedOperation ? retryFailedOperation : undefined}
          />
          <ExecutionTimeline runs={executionRuns} />
        </div>
      </div>
    );
  }

  if (phase === 'report' && report) {
    return <ReportView report={report} turns={turns} executionRuns={executionRuns} onReset={resetInterview} onExport={exportReview} exporting={exportingReview} />;
  }

  return (
    <div className="flex-1 overflow-y-auto bg-slate-50/70 px-4 py-5 sm:px-6">
      <div className="mx-auto max-w-6xl space-y-4">
        <InterviewTopbar progress={progress} config={config} onReset={resetInterview} onExport={exportReview} exporting={exportingReview} canExport={turns.length > 0} />
        <StatusMessage
          error={error}
          busy={busy}
          label={busyLabel}
          onRetry={canRetryTranscription
            ? () => void analyzeCachedVoiceRecording()
            : failedOperation ? retryFailedOperation : undefined}
          retryLabel={canRetryTranscription ? '重新分析录音' : undefined}
        />
        <ExecutionTimeline runs={executionRuns} />

        {phase === 'questioning' && question && (
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
            <main className="min-w-0 space-y-4">
              <section className="rounded-lg border border-slate-200 bg-white p-5 shadow-card sm:p-6">
                <QuestionHeader
                  question={question}
                  speechPhase={speechPhase}
                  onSpeak={() => toggleQuestionSpeech(question.text)}
                />
                <p className="mt-5 text-lg font-semibold leading-8 text-primary">{question.text}</p>
                {question.adaptation && (
                  <div className="mt-4 flex items-start gap-2 border-l-2 border-amber-400 bg-amber-50/70 px-3 py-2 text-xs leading-5 text-amber-800">
                    <Crosshair size={13} className="mt-1 shrink-0" />
                    <span>{question.adaptation.basedOn}</span>
                  </div>
                )}
              </section>

              <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-card sm:p-5">
                <div className="mb-3 flex items-center justify-between">
                  <label htmlFor="interview-answer" className="text-sm font-semibold text-primary">你的回答</label>
                  <span className="text-[11px] text-slate-400">{answer.length} 字</span>
                </div>
                <textarea
                  id="interview-answer"
                  value={answer}
                  onChange={(event) => setAnswer(event.target.value)}
                  onKeyDown={(event) => {
                    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') void submitAnswer(answer);
                  }}
                  rows={9}
                  autoFocus
                  placeholder="直接回答。面试官会核对事实、项目归属、指标与技术取舍。"
                  className="w-full resize-none rounded-md border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-700 outline-none focus:border-accent/60 focus:bg-white focus:ring-2 focus:ring-accent/10"
                />
                <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                  <button
                    type="button"
                    onClick={() => void (isRecording ? stopRecording() : startRecording())}
                    disabled={busy}
                    className={`flex h-10 items-center gap-2 rounded-md px-4 text-xs font-semibold ${
                      isRecording ? 'border border-red-200 bg-red-50 text-red-600' : 'border border-slate-200 text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    {isRecording ? <Square size={14} /> : <Mic size={14} />}
                    {isRecording ? '停止并分析' : '语音回答'}
                  </button>
                  <button
                    type="button"
                    onClick={() => void submitAnswer(answer)}
                    disabled={!answer.trim() || busy}
                    className="flex h-10 items-center gap-2 rounded-md bg-accent px-5 text-xs font-semibold text-white hover:bg-accent-dark disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    {busy ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
                    提交回答
                  </button>
                </div>
              </section>
            </main>

            <EvidenceRail question={question} profile={profile} turns={turns} />
          </div>
        )}

        {phase === 'feedback' && question && feedback && (
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
            <main className="min-w-0 space-y-4">
              <section className="rounded-lg border border-slate-200 bg-white p-5 shadow-card sm:p-6">
                <div className="flex flex-col gap-4 border-b border-slate-100 pb-5 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <VerdictBadge verdict={feedback.verdict} />
                    <h3 className="mt-3 text-base font-semibold text-primary">{feedback.summary}</h3>
                  </div>
                  {!feedback.deferred && (
                    <div className="flex h-20 w-20 shrink-0 flex-col items-center justify-center rounded-lg border border-slate-200 bg-slate-50">
                      <span className="text-3xl font-bold text-primary">{feedback.score}</span>
                      <span className="text-[10px] text-slate-400">/ 100</span>
                    </div>
                  )}
                </div>

                {!feedback.deferred && feedback.correction && (
                  <div className="mt-4 border-l-2 border-red-500 bg-red-50 px-4 py-3">
                    <div className="mb-1 text-xs font-semibold text-red-700">事实纠正</div>
                    <p className="text-xs leading-5 text-red-700">{feedback.correction}</p>
                  </div>
                )}

                {!feedback.deferred && (
                  <div className="mt-5 grid gap-5 md:grid-cols-2">
                    <FeedbackList title="站得住的部分" items={feedback.strengths} positive />
                    <FeedbackList title="继续追的漏洞" items={feedback.gaps} />
                  </div>
                )}

                {!feedback.deferred && feedback.claimChecks.length > 0 && (
                  <div className="mt-5 border-t border-slate-100 pt-4">
                    <h4 className="mb-3 text-xs font-semibold text-slate-500">简历陈述核对</h4>
                    <div className="space-y-2">
                      {feedback.claimChecks.map((check, index) => (
                        <div key={`${check.claim}-${index}`} className="flex items-start gap-2 text-xs leading-5">
                          {check.verdict === 'supported' ? (
                            <Check size={14} className="mt-0.5 shrink-0 text-emerald-500" />
                          ) : check.verdict === 'contradicted' ? (
                            <X size={14} className="mt-0.5 shrink-0 text-red-500" />
                          ) : (
                            <AlertCircle size={14} className="mt-0.5 shrink-0 text-amber-500" />
                          )}
                          <span className="min-w-0 text-slate-600">
                            <span className={`mr-2 inline-block font-semibold ${claimVerdictClass(check.verdict)}`}>
                              {claimVerdictLabel(check.verdict)}
                            </span>
                            {check.claim}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {!feedback.deferred && (
                  <div className="mt-5 rounded-md bg-sky-50 px-4 py-3 text-xs leading-5 text-sky-800">
                    <span className="font-semibold">下一次这样答：</span> {feedback.coachTip}
                  </div>
                )}
              </section>

              <div className="flex justify-end">
                <button
                  type="button"
                  onClick={continueInterview}
                  disabled={busy}
                  className="flex h-11 items-center gap-2 rounded-md bg-primary px-5 text-sm font-semibold text-white hover:bg-primary-light disabled:opacity-40"
                >
                  {busy ? <Loader2 size={15} className="animate-spin" /> : pendingQuestion ? <ArrowRight size={15} /> : <BarChart3 size={15} />}
                  {pendingQuestion ? nextActionLabel(pendingQuestion) : '生成面试报告'}
                </button>
              </div>
            </main>

            <aside className="space-y-4">
              {pendingQuestion && (
                <section className="rounded-lg border border-amber-200 bg-amber-50/60 p-4">
                  <div className="mb-2 flex items-center gap-2 text-xs font-semibold text-amber-800">
                    <Crosshair size={14} />
                    下一步策略
                  </div>
                  <p className="text-xs leading-5 text-amber-800">
                    {pendingQuestion.adaptation?.basedOn || '切换到尚未覆盖的岗位能力。'}
                  </p>
                  <div className="mt-3 flex items-center gap-2 text-[11px] text-amber-700">
                    <span>深度 L{pendingQuestion.depth}</span>
                    <span className="text-amber-300">/</span>
                    <span>{questionKindLabel(pendingQuestion.kind, pendingQuestion.focus)}</span>
                  </div>
                </section>
              )}
              <EvidenceRail question={question} profile={profile} turns={turns} compact />
            </aside>
          </div>
        )}
      </div>
    </div>
  );
}

function ControlGroup({ label, icon: Icon, children }: { label: string; icon: typeof Target; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-slate-500">
        <Icon size={13} />
        {label}
      </div>
      {children}
    </div>
  );
}

function StatusMessage({
  error,
  busy,
  label,
  onRetry,
  retryLabel = '重试本步',
}: {
  error: string | null;
  busy: boolean;
  label: string;
  onRetry?: () => void;
  retryLabel?: string;
}) {
  if (!error && !busy) return null;
  return (
    <div className={`flex min-h-10 items-center gap-2 rounded-md px-3 py-2 text-xs ${
      error ? 'border border-red-200 bg-red-50 text-red-700' : 'border border-sky-200 bg-sky-50 text-sky-700'
    }`}>
      {error ? <AlertCircle size={14} className="shrink-0" /> : <Loader2 size={14} className="shrink-0 animate-spin" />}
      <span className="min-w-0 flex-1">{error || label}</span>
      {error && onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="flex h-7 shrink-0 items-center gap-1 rounded border border-red-200 bg-white px-2.5 text-[11px] font-semibold text-red-700 hover:bg-red-100"
        >
          <RotateCcw size={11} />
          {retryLabel}
        </button>
      )}
    </div>
  );
}

function InterviewTopbar({ progress, config, onReset, onExport, exporting, canExport }: {
  progress: InterviewProgress;
  config: InterviewConfig;
  onReset: () => void;
  onExport: () => void;
  exporting: boolean;
  canExport: boolean;
}) {
  return (
    <header className="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-card">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary text-white">
          <Crosshair size={17} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="font-semibold text-primary">第 {Math.max(progress.current, progress.answered, 1)} 轮</span>
            <span className="text-slate-400">已完成 {progress.answered} / {progress.target}</span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100">
            <div className="h-full rounded-full bg-accent transition-[width]" style={{ width: `${Math.min(100, progress.percent)}%` }} />
          </div>
        </div>
        <div className="hidden shrink-0 items-center gap-2 text-[11px] text-slate-500 sm:flex">
          <span className="rounded bg-slate-100 px-2 py-1">{focusLabel(config.focus)}</span>
          <span className="rounded bg-slate-100 px-2 py-1">{difficultyLabel(config.difficulty)}</span>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            title="导出截至当前的完整复盘"
            onClick={onExport}
            disabled={!canExport || exporting}
            className="flex h-9 items-center gap-1.5 rounded-md border border-emerald-200 bg-emerald-50 px-3 text-xs font-semibold text-emerald-700 hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {exporting ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
            <span className="hidden sm:inline">导出复盘</span>
          </button>
          <button type="button" title="结束并重新设置" onClick={onReset} className="flex h-9 w-9 items-center justify-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-primary">
            <RotateCcw size={15} />
          </button>
        </div>
      </div>
    </header>
  );
}

function QuestionHeader({
  question,
  speechPhase,
  onSpeak,
}: {
  question: InterviewQuestion;
  speechPhase: QuestionSpeechPhase;
  onSpeak: () => void;
}) {
  const isLoading = speechPhase === 'loading';
  const isSpeaking = speechPhase === 'speaking';
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="rounded bg-primary px-2 py-1 text-[11px] font-semibold text-white">Q{question.index}</span>
      <span className="rounded bg-slate-100 px-2 py-1 text-[11px] font-medium text-slate-600">{questionKindLabel(question.kind, question.focus)}</span>
      <span className="rounded bg-sky-50 px-2 py-1 text-[11px] font-medium text-sky-700">{question.topic}</span>
      <span className="text-[11px] text-slate-400">深度 L{question.depth}/{question.maxDepth}</span>
      <button
        type="button"
        title={isLoading || isSpeaking ? '停止朗读' : '朗读问题'}
        aria-label={isLoading || isSpeaking ? '停止朗读' : '朗读问题'}
        onClick={onSpeak}
        className="ml-auto flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-accent"
      >
        {isLoading ? (
          <Loader2 size={15} className="animate-spin text-accent" />
        ) : isSpeaking ? (
          <Square size={14} className="text-accent" />
        ) : (
          <Volume2 size={15} />
        )}
      </button>
    </div>
  );
}

function EvidenceRail({
  question,
  profile,
  turns,
  compact = false,
}: {
  question: InterviewQuestion;
  profile: CandidateProfile | null;
  turns: InterviewTurn[];
  compact?: boolean;
}) {
  return (
    <aside className={`space-y-4 ${compact ? '' : 'lg:sticky lg:top-4 lg:self-start'}`}>
      <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-card">
        <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-primary">
          <Database size={14} className="text-accent" />
          本题证据
        </div>
        <div className="space-y-3">
          {question.evidenceRefs.length > 0 ? question.evidenceRefs.map((ref) => (
            <div key={ref.id} className="border-l-2 border-slate-200 pl-3">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-slate-600">
                <EvidenceIcon source={ref.source} />
                {ref.label}
              </div>
              {ref.excerpt && <p className="line-clamp-4 text-[11px] leading-5 text-slate-500">{ref.excerpt}</p>}
            </div>
          )) : (
            <p className="text-xs text-slate-400">本题没有可展示的证据引用。</p>
          )}
        </div>
      </section>

      {!compact && profile && (
        <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-card">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-primary">
            <Target size={14} className="text-amber-500" />
            候选人画像
          </div>
          {profile.targetRole && <p className="mb-3 text-xs font-medium text-slate-700">{profile.targetRole}</p>}
          <TagList label="项目" values={profile.projects} />
          <TagList label="能力" values={profile.topics} />
        </section>
      )}

      {!compact && turns.length > 0 && (
        <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-card">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-primary">
            <Activity size={14} className="text-emerald-500" />
            已识别风险
          </div>
          <div className="space-y-2">
            {turns.flatMap((turn) => turn.feedback.gaps).slice(-4).map((gap, index) => (
              <div key={`${gap}-${index}`} className="flex items-start gap-2 text-[11px] leading-5 text-slate-500">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400" />
                {gap}
              </div>
            ))}
          </div>
        </section>
      )}
    </aside>
  );
}

function EvidenceIcon({ source }: { source: 'jd' | 'resume' | 'knowledge' | 'answer' }) {
  if (source === 'jd') return <BriefcaseBusiness size={12} className="text-amber-500" />;
  if (source === 'resume') return <FileText size={12} className="text-sky-500" />;
  if (source === 'knowledge') return <Database size={12} className="text-emerald-500" />;
  return <Activity size={12} className="text-slate-400" />;
}

function TagList({ label, values }: { label: string; values: string[] }) {
  if (!values.length) return null;
  return (
    <div className="mb-3 last:mb-0">
      <div className="mb-1.5 text-[10px] font-semibold uppercase text-slate-400">{label}</div>
      <div className="flex flex-wrap gap-1.5">
        {values.slice(0, 6).map((value) => (
          <span key={value} className="max-w-full truncate rounded bg-slate-100 px-2 py-1 text-[11px] text-slate-600">{value}</span>
        ))}
      </div>
    </div>
  );
}

function VerdictBadge({ verdict }: { verdict: InterviewFeedback['verdict'] }) {
  const config = {
    strong: ['回答扎实', 'bg-emerald-50 text-emerald-700'],
    partial: ['部分成立', 'bg-amber-50 text-amber-700'],
    weak: ['存在漏洞', 'bg-red-50 text-red-700'],
    off_topic: ['偏离问题', 'bg-slate-100 text-slate-600'],
    deferred: ['报告汇总', 'bg-slate-100 text-slate-600'],
  }[verdict];
  return <span className={`inline-flex rounded px-2 py-1 text-[11px] font-semibold ${config[1]}`}>{config[0]}</span>;
}

function FeedbackList({ title, items, positive = false }: { title: string; items: string[]; positive?: boolean }) {
  return (
    <div>
      <h4 className="mb-2 text-xs font-semibold text-slate-500">{title}</h4>
      {items.length > 0 ? (
        <ul className="space-y-2">
          {items.map((item, index) => (
            <li key={`${item}-${index}`} className="flex items-start gap-2 text-xs leading-5 text-slate-600">
              {positive ? (
                <Check size={14} className="mt-0.5 shrink-0 text-emerald-500" />
              ) : (
                <X size={14} className="mt-0.5 shrink-0 text-red-400" />
              )}
              <span>{item}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-slate-400">无</p>
      )}
    </div>
  );
}

function ReportView({
  report,
  turns,
  executionRuns,
  onReset,
  onExport,
  exporting,
}: {
  report: InterviewReport;
  turns: InterviewTurn[];
  executionRuns: InterviewExecutionRun[];
  onReset: () => void;
  onExport: () => void;
  exporting: boolean;
}) {
  return (
    <div className="flex-1 overflow-y-auto bg-slate-50/70 px-4 py-5 sm:px-6">
      <div className="mx-auto max-w-6xl space-y-5">
        <header className="flex flex-col gap-4 border-b border-slate-200 pb-5 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase text-slate-400">
              <ShieldCheck size={13} className="text-emerald-500" />
              Evidence-backed report
            </div>
            <h2 className="text-xl font-bold text-primary">面试报告</h2>
            <p className="mt-1 max-w-2xl text-sm text-slate-500">{report.summary}</p>
          </div>
          <div className="flex gap-2 self-start">
            <button type="button" onClick={onExport} disabled={exporting} className="flex h-10 items-center gap-2 rounded-md bg-emerald-700 px-4 text-xs font-semibold text-white hover:bg-emerald-800 disabled:opacity-50">
              {exporting ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
              导出复盘
            </button>
            <button type="button" onClick={onReset} className="flex h-10 items-center gap-2 rounded-md border border-slate-200 bg-white px-4 text-xs font-semibold text-slate-600 hover:border-accent hover:text-accent">
              <RotateCcw size={14} />新面试
            </button>
          </div>
        </header>

        <ExecutionTimeline runs={executionRuns} />

        <div className="grid gap-4 lg:grid-cols-[260px_minmax(0,1fr)]">
          <aside className="space-y-4">
            <section className="rounded-lg border border-slate-200 bg-white p-5 text-center shadow-card">
              <div className="text-5xl font-bold text-primary">{report.overallScore}</div>
              <div className="mt-1 text-xs text-slate-400">已评估回答 / 100</div>
              <div className={`mx-auto mt-4 inline-flex rounded px-2.5 py-1 text-xs font-semibold ${
                report.readiness === 'ready' ? 'bg-emerald-50 text-emerald-700' :
                  report.readiness === 'borderline' ? 'bg-amber-50 text-amber-700' : 'bg-red-50 text-red-700'
              }`}>
                {report.readiness === 'ready' ? '可以上场' : report.readiness === 'borderline' ? '仍需补强' : '暂未就绪'}
              </div>
            </section>
            <ReportList title="已证明优势" items={report.strengths} positive />
            <ReportList title="高风险项" items={report.risks} />
            <ReportList title="下一轮训练" items={report.nextDrills} neutral />
          </aside>

          <main className="min-w-0 space-y-4">
            <section className="rounded-lg border border-slate-200 bg-white p-5 shadow-card">
              <h3 className="mb-4 text-sm font-semibold text-primary">能力维度</h3>
              <div className="grid gap-4 sm:grid-cols-2">
                {report.dimensions.map((dimension) => (
                  <div key={dimension.key} className="border-b border-slate-100 pb-4 last:border-0 sm:last:border-b">
                    <div className="mb-2 flex items-center justify-between text-xs">
                      <span className="font-semibold text-slate-600">{dimension.label}</span>
                      <span className={`font-bold ${dimension.assessed ? 'text-primary' : 'text-slate-400'}`}>
                        {dimension.assessed ? dimension.score : '未评估'}
                      </span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-slate-100">
                      {dimension.assessed && (
                        <div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(100, dimension.score)}%` }} />
                      )}
                    </div>
                    <p className="mt-2 text-[11px] leading-5 text-slate-500">{dimension.summary}</p>
                  </div>
                ))}
              </div>
            </section>

            <section className="rounded-lg border border-slate-200 bg-white p-5 shadow-card">
              <div className="mb-4 flex items-center gap-2">
                <Target size={14} className="text-amber-500" />
                <h3 className="text-sm font-semibold text-primary">覆盖审计</h3>
              </div>
              <div className="grid gap-5 md:grid-cols-2">
                <div className="min-w-0">
                  <h4 className="mb-2 text-[11px] font-semibold uppercase text-slate-400">JD 要求</h4>
                  <div className="divide-y divide-slate-100 border-y border-slate-100">
                    {report.jdCoverage.length ? report.jdCoverage.map((item, index) => (
                      <div key={`${item.requirement}-${index}`} className="flex items-start gap-3 py-3">
                        <span className={`mt-0.5 shrink-0 rounded px-2 py-0.5 text-[10px] font-semibold ${coverageStatusClass(item.status)}`}>
                          {coverageStatusLabel(item.status)}
                        </span>
                        <div className="min-w-0">
                          <p className="text-xs leading-5 text-slate-600">{item.requirement}</p>
                          {item.evidence.length > 0 && (
                            <p className="mt-0.5 text-[10px] text-slate-400">关联 {item.evidence.length} 轮回答</p>
                          )}
                        </div>
                      </div>
                    )) : <p className="py-3 text-xs text-slate-400">未提供 JD 材料</p>}
                  </div>
                </div>

                <div className="min-w-0">
                  <h4 className="mb-2 text-[11px] font-semibold uppercase text-slate-400">简历项目</h4>
                  <div className="divide-y divide-slate-100 border-y border-slate-100">
                    {report.projectCoverage.length ? report.projectCoverage.map((item, index) => (
                      <div key={`${item.project}-${index}`} className="py-3">
                        <div className="flex items-start justify-between gap-3">
                          <p className="min-w-0 text-xs leading-5 text-slate-600">{item.project}</p>
                          <span className={`shrink-0 text-[10px] font-semibold ${item.depth > 0 ? 'text-emerald-700' : 'text-slate-400'}`}>
                            {item.depth > 0 ? `深挖 ${item.depth} 轮` : '未覆盖'}
                          </span>
                        </div>
                        {item.risks.length > 0 && (
                          <p className="mt-1 line-clamp-2 text-[10px] leading-4 text-red-500">{item.risks[0]}</p>
                        )}
                      </div>
                    )) : <p className="py-3 text-xs text-slate-400">未提供简历项目</p>}
                  </div>
                </div>
              </div>
            </section>

            <section className="rounded-lg border border-slate-200 bg-white p-5 shadow-card">
              <h3 className="mb-4 text-sm font-semibold text-primary">逐轮证据</h3>
              <div className="space-y-5">
                {(report.turns.length ? report.turns : turns).map((turn, index) => (
                  <article key={turn.question.id} className="border-l-2 border-slate-200 pl-4">
                    <div className="mb-2 flex flex-wrap items-center gap-2">
                      <span className="text-[11px] font-bold text-accent">Q{index + 1}</span>
                      <span className="rounded bg-slate-100 px-2 py-0.5 text-[10px] text-slate-500">{turn.question.topic}</span>
                      <span className="ml-auto text-xs font-semibold text-primary">{turn.feedback.score}</span>
                    </div>
                    <p className="text-xs font-medium leading-5 text-slate-700">{turn.question.text}</p>
                    <p className="mt-2 line-clamp-3 text-xs leading-5 text-slate-500">{turn.answer}</p>
                    {turn.feedback.gaps.length > 0 && (
                      <p className="mt-2 text-[11px] leading-5 text-red-600">风险：{turn.feedback.gaps.join('；')}</p>
                    )}
                  </article>
                ))}
              </div>
            </section>
          </main>
        </div>
      </div>
    </div>
  );
}

function ReportList({ title, items, positive = false, neutral = false }: { title: string; items: string[]; positive?: boolean; neutral?: boolean }) {
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-card">
      <h3 className="mb-3 text-xs font-semibold text-primary">{title}</h3>
      <ul className="space-y-2">
        {items.length ? items.map((item, index) => (
          <li key={`${item}-${index}`} className="flex items-start gap-2 text-[11px] leading-5 text-slate-500">
            {positive ? <Check size={13} className="mt-0.5 shrink-0 text-emerald-500" /> :
              neutral ? <ArrowRight size={13} className="mt-0.5 shrink-0 text-accent" /> :
                <AlertCircle size={13} className="mt-0.5 shrink-0 text-red-400" />}
            <span>{item}</span>
          </li>
        )) : <li className="text-[11px] text-slate-400">无</li>}
      </ul>
    </section>
  );
}

function interviewErrorMessage(errorValue: unknown) {
  if (errorValue instanceof InterviewRequestError) {
    if (errorValue.code === 'service_unavailable') {
      return '评估 Agent 暂时不可用或执行超时，本次内容没有提交。已完成轨迹会保留，可以直接重试。';
    }
    if (errorValue.code === 'backend_unavailable') {
      return 'Go Harness 当前不可达，本次内容没有提交。请稍后重试。';
    }
    if (errorValue.code === 'stream_interrupted') {
      return '执行轨迹连接中断，本次结果未确认。请重试当前步骤。';
    }
    if (errorValue.code === 'conflict' || errorValue.code === 'invalid_state') {
      return '面试状态已经变化，请返回当前题目后重新提交。';
    }
    return errorValue.message;
  }
  const message = errorValue instanceof Error ? errorValue.message : String(errorValue);
  if (message.includes('interview assessment is temporarily unavailable')) {
    return '评估 Agent 暂时不可用或执行超时，本次回答没有提交。可以直接重试。';
  }
  return message || '面试执行失败，本次内容没有提交。';
}

function waitForRecoveryPoll(delayMs: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Recovery cancelled', 'AbortError'));
      return;
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', cancel);
      resolve();
    }, delayMs);
    const cancel = () => {
      window.clearTimeout(timer);
      reject(new DOMException('Recovery cancelled', 'AbortError'));
    };
    signal.addEventListener('abort', cancel, { once: true });
  });
}

function focusLabel(focus: InterviewFocus) {
  return focusOptions.find((option) => option.value === focus)?.label ?? focus;
}

function difficultyLabel(difficulty: InterviewDifficulty) {
  return difficultyOptions.find((option) => option.value === difficulty)?.label ?? difficulty;
}

function questionKindLabel(kind: InterviewQuestion['kind'], focus?: InterviewQuestion['focus']) {
  return {
    opening: '新覆盖点',
    follow_up: '连续追问',
    challenge: '反事实挑战',
    verification: focus === 'knowledge' ? '基础纠错' : '陈述核对',
  }[kind];
}

function nextActionLabel(question: InterviewQuestion) {
  if (question.kind === 'follow_up') return '继续深挖';
  if (question.kind === 'verification') return question.focus === 'knowledge' ? '校正基础理解' : '核对项目陈述';
  if (question.kind === 'challenge') return '进入压力追问';
  return '进入下一覆盖点';
}

function coverageStatusLabel(status: InterviewReport['jdCoverage'][number]['status']) {
  return { covered: '已覆盖', partial: '部分', missing: '缺失' }[status];
}

function coverageStatusClass(status: InterviewReport['jdCoverage'][number]['status']) {
  return {
    covered: 'bg-emerald-50 text-emerald-700',
    partial: 'bg-amber-50 text-amber-700',
    missing: 'bg-slate-100 text-slate-500',
  }[status];
}

function claimVerdictLabel(verdict: InterviewFeedback['claimChecks'][number]['verdict']) {
  return {
    supported: '材料内支持',
    unverified: '尚未验证',
    contradicted: '与材料矛盾',
    not_in_material: '材料外新增',
  }[verdict];
}

function claimVerdictClass(verdict: InterviewFeedback['claimChecks'][number]['verdict']) {
  return {
    supported: 'text-emerald-700',
    unverified: 'text-amber-700',
    contradicted: 'text-red-700',
    not_in_material: 'text-amber-700',
  }[verdict];
}

function encodeWav(chunks: Float32Array[], sourceRate: number): Blob {
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const merged = new Float32Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }
  const targetRate = 16000;
  const samples = downsample(merged, sourceRate, targetRate);
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  writeAscii(view, 0, 'RIFF');
  view.setUint32(4, 36 + samples.length * 2, true);
  writeAscii(view, 8, 'WAVE');
  writeAscii(view, 12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, targetRate, true);
  view.setUint32(28, targetRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeAscii(view, 36, 'data');
  view.setUint32(40, samples.length * 2, true);
  let cursor = 44;
  for (const sample of samples) {
    const clamped = Math.max(-1, Math.min(1, sample));
    view.setInt16(cursor, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true);
    cursor += 2;
  }
  return new Blob([buffer], { type: 'audio/wav' });
}

function downsample(input: Float32Array, sourceRate: number, targetRate: number): Float32Array {
  if (sourceRate === targetRate) return input;
  const ratio = sourceRate / targetRate;
  const output = new Float32Array(Math.max(1, Math.round(input.length / ratio)));
  for (let i = 0; i < output.length; i++) {
    const start = Math.floor(i * ratio);
    const end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = start; j < end; j++) sum += input[j];
    output[i] = sum / Math.max(1, end - start);
  }
  return output;
}

function writeAscii(view: DataView, offset: number, text: string) {
  for (let i = 0; i < text.length; i++) view.setUint8(offset + i, text.charCodeAt(i));
}
