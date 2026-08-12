import { describe, expect, it } from 'vitest';
import { buildInterviewReviewHtml } from '../../web/src/lib/interview-review-export';
import type { InterviewReviewSnapshot } from '../../web/src/types/interview';

describe('interview review HTML export', () => {
  it('embeds recordings and safe traces while escaping candidate content', () => {
    const review: InterviewReviewSnapshot = {
      schemaVersion: '1.0.0', interviewId: 'interview-1', state: 'questioning',
      startedAt: '2026-08-13T00:00:00Z', generatedAt: '2026-08-13T00:01:00Z',
      turns: [{
        question: { id: 'q1', index: 1, text: '如何设计？', kind: 'opening', focus: 'knowledge', topic: '架构', difficulty: 'hard', depth: 1, maxDepth: 3, evidenceRefs: [] },
        answer: '<script>alert(1)</script>', inputMode: 'voice', durationMs: 2500,
        feedback: { questionId: 'q1', score: 25, verdict: 'weak', summary: '缺少边界', strengths: ['有方向'], gaps: ['无接口'], claimChecks: [], coachTip: '先画边界' },
        references: [{ evidenceId: 'k1', title: '参考题', answer: '标准答案' }], answeredAt: '2026-08-13T00:00:30Z',
      }],
    };
    const html = buildInterviewReviewHtml({
      review,
      recordings: [{ questionId: 'q1', dataUrl: 'data:audio/wav;base64,UklGRg==', mimeType: 'audio/wav', durationMs: 2500 }],
      executionRuns: [{ id: 'r1', action: 'answer', title: '第 1 轮回答评估', status: 'completed', startedAt: '2026-08-13T00:00:30Z', steps: [{ id: 's1', stage: 'assessment', label: '评估完成', detail: '安全摘要', status: 'completed', at: '2026-08-13T00:00:31Z' }] }],
    });

    expect(html).toContain('data:audio/wav;base64,UklGRg==');
    expect(html).toContain('标准答案');
    expect(html).toContain('评估完成');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
    expect(html).not.toContain('<script>alert(1)</script>');
    expect(html).toContain('Schema');
  });
});
