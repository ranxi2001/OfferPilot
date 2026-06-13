export { ToolRegistry } from './registry.js';
export type { ToolDefinition, ToolContext, ToolResult, RiskLevel } from './types.js';

import { ToolRegistry } from './registry.js';
import { searchKnowledge } from './builtin/search-knowledge.js';
import { diagnoseAnswer } from './builtin/diagnose-answer.js';
import { generateFollowup } from './builtin/generate-followup.js';
import { scoreRubric } from './builtin/score-rubric.js';
import { compareAnswers } from './builtin/compare-answers.js';
import { listDimensions } from './builtin/list-dimensions.js';
import { sessionReport } from './builtin/session-report.js';
import { suggestStudyPath } from './builtin/suggest-study-path.js';
import { analyzeJd } from './builtin/analyze-jd.js';
import { matchResumeJd } from './builtin/match-resume-jd.js';
import { optimizeResume } from './builtin/optimize-resume.js';
import { mockInterview } from './builtin/mock-interview.js';
import { realtimeInterview } from './builtin/realtime-interview.js';

export function createToolRegistry(): ToolRegistry {
  const registry = new ToolRegistry();

  // Interview diagnosis tools
  registry.register(searchKnowledge);
  registry.register(diagnoseAnswer);
  registry.register(generateFollowup);
  registry.register(scoreRubric);
  registry.register(compareAnswers);
  registry.register(listDimensions);
  registry.register(sessionReport);
  registry.register(suggestStudyPath);

  // JD analysis & resume tools
  registry.register(analyzeJd);
  registry.register(matchResumeJd);
  registry.register(optimizeResume);
  registry.register(mockInterview);

  // Realtime interview simulation
  registry.register(realtimeInterview);

  return registry;
}
