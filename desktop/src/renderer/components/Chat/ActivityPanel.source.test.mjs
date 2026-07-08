import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const panelPath = path.join(__dirname, 'ActivityPanel.tsx');
const stylesPath = path.resolve(__dirname, '../../styles/index.css');
const source = readFileSync(panelPath, 'utf8');
const stylesSource = readFileSync(stylesPath, 'utf8');

test('activity panel keeps streaming text and steps fully expanded', () => {
  assert.match(source, /import \{ ActivityMarkdownView \} from '\.\/ActivityMarkdown';/);
  assert.match(source, /function StreamingTextView\(\{[\s\S]*?text,[\s\S]*?participant,[\s\S]*?\}: \{[\s\S]*?text: string;[\s\S]*?participant: ActivityMessageParticipant;[\s\S]*?\}\) \{/);
  assert.match(source, /op-activity-panel-message-bubble is-assistant is-streaming/);
  assert.match(source, /function EntryMessageView\(\{[\s\S]*?message,[\s\S]*?participant,[\s\S]*?onAction,[\s\S]*?pendingActionID,/);
  assert.match(source, /getLiveStepSummary,/);
  assert.match(source, /const showMarkdownDetail = step\.type !== 'toolcall' && step\.status !== 'running';/);
  assert.match(source, /const summary = getLiveStepSummary\(step\);/);
  assert.match(source, /const showSummary = Boolean\(summary && summary !== step\.label && \(isToolcall \|\| step\.type === 'reasoning'\)\);/);
  assert.match(source, /<span className="op-activity-step-summary">\{summary\}<\/span>/);
  assert.match(source, /op-activity-panel-step-section-label">Input/);
  assert.match(source, /op-activity-panel-step-section-label">Output/);
  assert.match(source, /const retainedAnswerText = useMemo\(/);
  assert.match(source, /durableTimelineItems\.length > 0/);
  assert.match(source, /const timelineStreamingSegments = streamingSegments\.length > 0/);
  assert.match(source, /const liveTimelineSteps = isHistoricalWindow \? \[\] : steps;/);
  assert.match(source, /<EntryMessageView\s*key=\{item\.message\.id\}[\s\S]*?message=\{item\.message\}[\s\S]*?participant=\{resolveActivityMessageParticipant\(item\.message, activityMessageParticipants\)\}[\s\S]*?onAction=\{handleMessageAction\}[\s\S]*?pendingActionID=\{pendingMessageActionID\}/);
  assert.match(source, /<StreamingTextView[\s\S]*?key=\{item\.segment\.id\}[\s\S]*?text=\{item\.segment\.text\}[\s\S]*?participant=\{activityMessageParticipants\.assistant\}/);
  assert.match(source, /<LiveStepRow[\s\S]*?key=\{step\.id\}[\s\S]*?step=\{step\}[\s\S]*?participant=\{isAgentActivityTimelineItem\(item\) \? activityMessageParticipants\.assistant : null\}[\s\S]*?showParticipantHeader=\{showParticipantHeader\}/);
  assert.doesNotMatch(source, /Collapse diff preview/);
  assert.doesNotMatch(source, /Expand diff preview/);
  assert.doesNotMatch(source, /Collapse review turn/);
  assert.doesNotMatch(source, /Expand review turn/);
});

test('activity panel renders message participants with user and agent avatars', () => {
  assert.match(source, /import \{ useAuthStore \} from '\.\.\/\.\.\/store\/authStore';/);
  assert.match(source, /import \{ buildInitials, initialsBackgroundColor \} from '\.\.\/avatarInitials';/);
  assert.match(source, /import \{ resolveLooseResourceUrl \} from '\.\.\/\.\.\/services\/resourceService';/);
  assert.match(source, /import \{ resolveUserAvatarSrc \} from '\.\.\/TitlebarUserAvatar';/);
  assert.match(source, /type ActivityMessageParticipant = \{/);
  assert.match(source, /function ActivityMessageAvatar\(\{ participant \}: \{ participant: ActivityMessageParticipant \}\) \{/);
  assert.match(source, /className=\{`op-activity-panel-message-avatar is-\$\{participant\.kind\}`\}/);
  assert.match(source, /void resolveLooseResourceUrl\(avatarSrc\)/);
  assert.match(source, /const activityMessageParticipants = useMemo<ActivityMessageParticipants>\(\(\) => \{/);
  assert.match(source, /const ensureAgentRecord = useAppStore\(\(s\) => s\.ensureAgentRecord\);/);
  assert.match(source, /void ensureAgentRecord\(activeAgentID\)/);
  assert.match(source, /const indexedAgent = activeAgentID \? resolveAgentByID\(activeAgentID\) : null;/);
  assert.match(source, /const recordAgent = resolvedAgentRecord\?\.agentID === activeAgentID \? resolvedAgentRecord : null;/);
  assert.match(source, /avatarSrc: authLoggedIn \? resolveActivityParticipantAvatarSrc\(authProfile\) : null,/);
  assert.match(source, /avatarSrc: \(recordAgent\?\.avatar \|\| indexedAgent\?\.avatar \|\| ''\)\.trim\(\) \|\| null,/);
  assert.match(source, /participant=\{activityMessageParticipants\.assistant\}/);
  assert.match(stylesSource, /\.op-activity-panel-message-avatar\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-message-author-column\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-message-avatar-img,/);
  assert.match(stylesSource, /\.op-activity-panel-message-stack\s*\{/);
});

test('activity panel renders names to the right of avatars with light small text', () => {
  assert.match(source, /<div className="op-activity-panel-message-author-column">\s*<ActivityMessageAvatar participant=\{participant\} \/>\s*<\/div>/);
  assert.match(source, /<div className="op-activity-panel-message-stack">\s*\{showParticipantHeader \? \(\s*<ActivityMessageHeader message=\{headerMessage\} participant=\{participant\} hideTitle=\{hideHeaderTitle\} \/>\s*\) : null\}/);
  assert.match(stylesSource, /\.op-activity-panel-message-author-column,\s*\n\.op-activity-panel-message-author-column-spacer\s*\{[^}]*width:\s*28px;/m);
  assert.match(stylesSource, /\.op-activity-panel-message-author-column\s*\{[^}]*align-items:\s*center;[^}]*justify-content:\s*flex-start;/m);
  assert.match(stylesSource, /\.op-activity-panel-message-meta\s*\{[^}]*font-size:\s*10px;[^}]*font-weight:\s*500;/m);
  assert.match(stylesSource, /\.op-activity-panel-message-author,[\s\S]*?max-width:\s*180px;/m);
});

test('activity panel groups consecutive agent timeline items under one avatar', () => {
  assert.match(source, /type ActivityTimelineRenderItem = \{/);
  assert.match(source, /function isAgentActivityTimelineItem\(item: ActivityTimelineItem\): boolean \{/);
  assert.match(source, /item\.message\.bubbleRole === 'assistant' \|\| item\.message\.role === 'agent'/);
  assert.match(source, /return item\.step\.type === 'toolcall' \|\| item\.step\.type === 'reasoning';/);
  assert.match(source, /function buildActivityTimelineRenderItems\(items: ActivityTimelineItem\[\]\): ActivityTimelineRenderItem\[\] \{/);
  assert.match(source, /let agentSegmentOpen = false;/);
  assert.match(source, /const showParticipantHeader = !agentSegmentOpen;\s*agentSegmentOpen = true;/);
  assert.match(source, /agentSegmentOpen = false;\s*return \{ item, showParticipantHeader: true \};/);
  assert.match(source, /const timelineRenderItems = buildActivityTimelineRenderItems\(timelineItems\);/);
  assert.match(source, /\{timelineRenderItems\.map\(\(\{ item, showParticipantHeader \}\) => \{/);
  assert.match(source, /showParticipantHeader=\{showParticipantHeader\}/);
});

test('activity panel renders user input on the left with a lightweight chat bubble', () => {
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-message-row\.is-user\s*\{[^}]*flex-direction:\s*row-reverse;/m);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-message-row\.is-user \.op-activity-panel-message-stack\s*\{[^}]*align-items:\s*flex-end;/m);
  assert.match(stylesSource, /\.op-activity-panel-message-bubble\.is-user\s*\{[^}]*border-color:\s*color-mix\(in srgb, var\(--op-glass-border\) 36%, transparent\);/m);
  assert.match(stylesSource, /\.op-activity-panel-message-bubble\.is-user\s*\{[^}]*background:\s*color-mix\(in srgb, var\(--color-editor-bg\) 90%, white 10%\);/m);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-message-bubble\.is-user\s*\{[^}]*var\(--color-accent\) 20%/m);
});

test('activity panel collapses long message bodies after ninety-nine visual lines', () => {
  assert.match(source, /const ACTIVITY_PANEL_MESSAGE_COLLAPSE_LINES = 99;/);
  assert.match(source, /const bodyWrapRef = useRef<HTMLDivElement \| null>\(null\);/);
  assert.match(source, /const \[bodyExpanded, setBodyExpanded\] = useState\(false\);/);
  assert.match(source, /const \[bodyCanCollapse, setBodyCanCollapse\] = useState\(false\);/);
  assert.match(source, /body\.scrollHeight > body\.clientHeight \+ 1/);
  assert.match(source, /new ResizeObserver\(measure\)/);
  assert.match(source, /shouldMeasureClamp \? 'is-collapse-measure' : ''/);
  assert.match(source, /shouldShowCollapsedState \? 'is-collapsed' : ''/);
  assert.match(source, /'--op-activity-panel-message-collapse-lines': ACTIVITY_PANEL_MESSAGE_COLLAPSE_LINES/);
  assert.match(source, /bodyExpanded \? 'Show less' : 'Show more'/);
  assert.match(stylesSource, /\.op-activity-panel-message-body-wrap\.is-collapse-measure\s*\{/);
  assert.match(stylesSource, /-webkit-line-clamp:\s*var\(--op-activity-panel-message-collapse-lines, 99\);/);
  assert.match(stylesSource, /\.op-activity-panel-message-toggle\s*\{/);
});

test('activity panel renders header hints outside the collapsible output preview', () => {
  assert.match(source, /const threadTailStatus = threadState\.tailStatus;/);
  assert.match(source, /const queuedMessageCount = queuedMessages\.steering\.length \+ queuedMessages\.followUp\.length;/);
  assert.match(source, /threadTailStatus,/);
  assert.match(source, /queuedMessageCount,/);
  assert.match(source, /headerViewModel\.hint && \(/);
  assert.match(source, /className="op-activity-panel-statusbar-hint"/);
});

test('plan execution model picker defaults to the configured Default Chat model', () => {
  assert.match(source, /resolveDefaultChatModelSelection\(modelsConfig\)\.modelKey/);
  assert.match(source, /modelKey: defaultChatModelKey/);
  assert.doesNotMatch(source, /modelKey: null,/);
});

test('activity panel orders durable entries before live streaming segments and steps', () => {
  assert.match(source, /type ActivityTimelineItem =\s*\|\s*\{ kind: 'message'; ts: number; order: number \| null; message: EntryMessage \}\s*\|\s*\{ kind: 'stream'; ts: number; order: number \| null; segment: LiveTextSegment \}/);
  assert.match(source, /const threadEntries = useMemo\(/);
  assert.match(source, /threadEntryTimelineItems\(threadEntries\)/);
  assert.match(source, /const recordItemIndex = new Map<string, number>\(\);/);
  assert.match(source, /message_update collapses into the original message_append position/);
  assert.match(source, /existingIndex !== undefined/);
  assert.match(source, /threadMessageRecordTimelineItems\(threadSnapshot, entryItems\.length \* 100\)/);
  assert.match(source, /order: durableTimelineItems\.length \* 100 \+ \(step\.order \?\? 0\)/);
  assert.match(source, /order: durableTimelineItems\.length \* 100 \+ segment\.order/);
  assert.match(source, /if \(a\.order != null && b\.order != null && a\.order !== b\.order\) \{\s*return a\.order - b\.order;\s*\}/);
  assert.match(source, /return getActivityTimelineKindRank\(a\.kind\) - getActivityTimelineKindRank\(b\.kind\);/);
});

test('activity panel renders messenger message records and actions through the shared timeline', () => {
  assert.match(source, /import type \{ ChatMessageRecord, ThreadEntry, ThreadSnapshot \}/);
  assert.match(source, /function collectEntryMessageRecordIDs\(entries: ThreadEntry\[\]\): Set<string> \{/);
  assert.match(source, /function threadMessageRecordTimelineItems\(snapshot: ThreadSnapshot \| null, startOrder: number\): ActivityTimelineItem\[\] \{/);
  assert.match(source, /Array\.isArray\(snapshot\?\.messageRecords\) \? snapshot\.messageRecords : \[\]/);
  assert.match(source, /actions: Array\.isArray\(record\.actions\) \? record\.actions : undefined/);
  assert.match(source, /questions\?: EntryMessageQuestion\[\];/);
  assert.match(source, /replyToMessageID\?: string;/);
  assert.match(source, /answers\?: EntryMessageQuestionAnswer\[\];/);
  assert.match(source, /\.\.\.\(options\?\.questions\?\.length \? \{ questions: options\.questions \} : \{\}\),/);
  assert.match(source, /\.\.\.\(options\?\.replyToMessageID \? \{ replyToMessageID: options\.replyToMessageID \} : \{\}\),/);
  assert.match(source, /const questions = Array\.isArray\(record\?\.questions\)/);
  assert.match(source, /const answers = Array\.isArray\(record\?\.answers\)/);
  assert.match(source, /questions,\s*\n\s*replyToMessageID: asString\(record\?\.replyToMessageID\) \|\| undefined,\s*\n\s*actionID: asString\(record\?\.actionID\) \|\| undefined,\s*\n\s*answers,/);
  assert.match(source, /bubbleRole: bubbleRoleForMessage\(role, kind\)/);
  assert.match(source, /function buildAnsweredRequestMap\(items: ActivityTimelineItem\[\]\): Record<string, boolean> \{/);
  assert.match(source, /function buildMessageRecordByID\(items: ActivityTimelineItem\[\]\): Record<string, EntryMessage> \{/);
  assert.match(source, /function buildRequestAnswerDetailsByID\(items: ActivityTimelineItem\[\]\): Record<string, EntryMessageQuestionAnswer\[\]> \{/);
  assert.match(source, /function EntryOpenRequestCard\(/);
  assert.match(source, /op-activity-panel-open-request/);
  assert.match(source, /const showOpenRequestCard = message\.bubbleRole === 'request' && showQuestionChoices;/);
  assert.match(source, /hideHeaderTitle=\{isRequestSurface\}/);
  assert.match(source, /showQuestionChoices && !showOpenRequestCard \? \(/);
  assert.match(source, /const isRequestReadonlySurface = showStructuredAnswerCard \|\| isResolvedRequest;/);
  assert.match(source, /const isRequestSurface = showOpenRequestCard \|\| isRequestReadonlySurface;/);
  assert.match(source, /showOpenRequestCard \? 'is-open-request is-request-card' : '',/);
  assert.match(source, /isRequestReadonlySurface \? 'is-request-card-readonly' : '',/);
  assert.doesNotMatch(source, /OP_SG_FROST_ON_ACTIVITY_BODY/);
  assert.match(source, /function RequestQuestionsReadonly\(/);
  assert.match(source, /op-activity-panel-request-answer-card/);
  assert.match(source, /op-activity-panel-request-resolved-summary/);
  assert.match(source, /requestAnswerDetailsByID/);
  assert.match(source, /replyToRequest=/);
  assert.match(source, /function activityPanelActionKeycap\(index: number\): string \{/);
  assert.match(source, /ACTIVITY_PANEL_ACTION_KEYCAPS\[index\]/);
  assert.match(source, /questions: Array\.isArray\(record\.questions\) \? record\.questions : undefined/);
  assert.match(source, /const requestAnswered = Boolean\(answeredRequest\);/);
  assert.match(source, /const messageCanReply = message\.status === 'open' && !requestAnswered;/);
  assert.match(source, /const showQuestionChoices = Boolean\(message\.questions\?\.length && messageCanReply\);/);
  assert.doesNotMatch(source, /messageCanReply \|\| requestAnswered \|\| message\.status === 'resolved'/);
  assert.match(source, /messageCanReply && message\.actions\?\.length/);
  assert.doesNotMatch(source, /showQuestionChoices \? \(/);
  assert.match(source, /className=\{\['op-activity-panel-choice-button', selected \? 'is-selected' : ''\]\.filter\(Boolean\)\.join\(' '\)\}/);
  assert.doesNotMatch(source, /is-unselected/);
  assert.match(source, /disabled=\{disabled\}/);
  assert.match(source, /<ol className="op-activity-panel-choice-list" aria-label="Message choices">/);
  assert.match(source, /className="op-activity-panel-choice-key" aria-hidden="true">\{keycap\}<\/span>/);
  assert.match(source, /op-activity-panel-choice-badge">Recommended<\/span>/);
  assert.match(source, /op-activity-panel-choice-badge">Selected<\/span>/);
  assert.match(source, /message\.questions\?\.length/);
  assert.match(source, /op-activity-panel-question-list/);
  assert.match(source, /const questionTitle = \(message\.title \|\| ''\)\.trim\(\);/);
  assert.match(source, /className="op-activity-panel-question-title">\{questionTitle\}<\/div>/);
  assert.match(source, /Other\.\.\./);
  assert.match(source, /messageCanReply && openOtherQuestionID === questionID/);
  assert.match(source, /answers: \[\{/);
  assert.match(source, /questionID: normalizedQuestionID/);
  assert.match(source, /other: true/);
  assert.match(source, /isRequestAlreadyClosedError\(error\)/);
  assert.match(source, /await refreshMessageRequestState\(message\);/);
  assert.doesNotMatch(source, /op-activity-panel-question-header/);
  assert.doesNotMatch(source, /question\.allowOther/);
  assert.doesNotMatch(source, /option\.tone/);
  assert.doesNotMatch(source, /optionToneClass/);
  assert.doesNotMatch(source, /op-activity-panel-choice-description/);
  assert.doesNotMatch(source, /op-activity-panel-message-actions/);
  assert.doesNotMatch(source, /EntryMessageActionReply/);
  assert.doesNotMatch(source, /__custom__/);
  assert.match(stylesSource, /\.op-activity-panel-question-title\s*\{/);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-question-header\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-open-request\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-choice-list\.is-request-decision \.op-activity-panel-choice-button:hover:not\(:disabled\)[^}]*background:\s*var\(--color-hover-bg\);/m);
  assert.match(source, /function EntryRequestAnswerCard\(/);
  assert.match(stylesSource, /\.op-activity-panel-request-resolved-summary\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-choice-list\s*\{[^}]*display:\s*grid;/m);
  assert.match(stylesSource, /\.op-activity-panel-choice-button\s*\{[^}]*grid-template-columns:\s*28px minmax\(0, 1fr\);/m);
  assert.match(stylesSource, /\.op-activity-panel-choice-button\s*\{[^}]*border:\s*0;/m);
  assert.match(stylesSource, /\.op-activity-panel-choice-button\s*\{[^}]*background:\s*transparent;/m);
  assert.match(stylesSource, /\.op-activity-panel-choice-list\.is-request-decision\.is-answered\s*\n\s*\.op-activity-panel-choice-button\.is-selected,/m);
  assert.match(stylesSource, /\.op-activity-panel-message-bubble\.is-request-card\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-message-bubble\.is-request-card-readonly\s*\{[^}]*border-radius:\s*10px;/m);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-choice-button\.is-unselected\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-choice-key\s*\{[^}]*border-radius:\s*5px;/m);
  assert.match(stylesSource, /\.op-activity-panel-choice-custom-form\s*\{/);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-choice-description\s*\{/);
  assert.match(source, /const replyMessenger = useAppStore\(\(s\) => s\.replyMessenger\);/);
  assert.match(source, /await replyMessenger\(\{/);
  assert.match(source, /actionID: normalizedActionID/);
});

test('activity panel renders thread snapshots as bounded message windows', () => {
  assert.match(source, /loadThreadSnapshotWindow,/);
  assert.match(source, /const canLoadOlderWindow = Boolean\(entryWindow\?\.hasBefore && firstEntryId\);/);
  assert.match(source, /const isHistoricalWindow = Boolean\(entryWindow\?\.hasAfter\);/);
  assert.match(source, /mode: 'before',\s*anchorId: firstEntryId,\s*limit: 200,/);
  assert.match(source, /mode: 'tail',\s*limit: 400,/);
  assert.match(source, /ACTIVITY_PANEL_LOAD_OLDER_THRESHOLD_PX/);
  assert.match(source, /Jump to latest/);
  assert.match(source, /Load earlier/);
});

test('activity panel maps canonical tool calls and results into merged activity steps', () => {
  assert.match(source, /type ToolStepIndex = Map<string, Extract<ActivityTimelineItem, \{ kind: 'step' \}>>;/);
  assert.match(source, /function upsertToolStep\(/);
  assert.match(source, /toolCallID \? \{ id: toolCallID, name \} : undefined/);
  assert.match(source, /toolOutput: output \|\| undefined/);
  assert.match(source, /const isToolResultRole = role === 'tool_result';/);
});

test('activity panel separates shell visibility from thread activity body rendering', () => {
  assert.match(source, /function ThreadActivityView\(\{/);
  assert.match(source, /forceVisible = false,/);
  assert.match(source, /showEmptyState=\{showThreadActivityEmptyState\}/);
  assert.match(source, /&& !forceVisible\) return null;/);
  assert.match(source, /Loading conversation\.\.\./);
});

test('review panels render above the streaming timeline', () => {
  assert.match(source, /const pendingReviews = useMemo\(\(\) => reviews\.filter\(isPendingReview\), \[reviews\]\);/);
  assert.match(source, /const completedReviews = useMemo\(\(\) => reviews\.filter\(\(review\) => !isPendingReview\(review\)\), \[reviews\]\);/);

  const pendingIndex = source.indexOf('title="Pending Review"');
  const timelineIndex = source.indexOf('{timelineRenderItems.map(({ item, showParticipantHeader }) => {');
  const historyIndex = source.indexOf('title="Review History"');

  assert.ok(pendingIndex >= 0, 'pending review panel should exist');
  assert.ok(timelineIndex >= 0, 'timeline render should exist');
  assert.ok(historyIndex >= 0, 'review history panel should exist');
  assert.ok(pendingIndex < timelineIndex, 'pending reviews should render before the timeline');
  assert.ok(historyIndex < timelineIndex, 'review history should render before the timeline');
});

test('review files do not render inline diff content in the activity panel', () => {
  assert.doesNotMatch(source, /\{file\.diff \|\| '\[No diff preview\]'\}/);
  assert.doesNotMatch(source, /file\.diff/);
  assert.match(source, /onClick=\{\(\) => onNavigateFile\(review, file\)\}/);
});

test('review file navigation passes structured overlay metadata to the editor', () => {
  assert.match(source, /threadID: review\.threadID,/);
  assert.match(source, /chatPath: review\.chatPath,/);
  assert.match(source, /hunks: file\.hunks \|\| \[\],/);
  assert.match(source, /\(file\.changedRanges \|\| \[\]\)\.length === 0 && \(file\.hunks \|\| \[\]\)\.length === 0/);
});

test('activity panel status chip uses unified static glass capsule classes', () => {
  assert.match(source, /OP_SG_CAPSULE/);
  assert.match(source, /OP_SG_CAPSULE_ON_ACTIVITY_HEADER/);
  assert.match(
    source,
    /\$\{OP_SG_CAPSULE\} \$\{OP_SG_CAPSULE_ON_ACTIVITY_HEADER\} op-activity-panel-status-chip is-\$\{headerViewModel\.tone\}/,
  );
});

test('activity panel body renders clickable thread metadata', () => {
  assert.match(source, /import \{ buildThreadLinkTarget \} from '\.\.\/\.\.\/utils\/threadLink';/);
  assert.match(source, /import \{ navigateFrontmatterLink \} from '\.\.\/\.\.\/utils\/frontmatterLinkNavigate';/);
  assert.match(source, /const activeThreadLinkTarget = buildThreadLinkTarget\(activeThreadID\);/);
  assert.match(
    source,
    /const handleActivityThreadIDClick = useCallback\(\(event: React\.MouseEvent<HTMLButtonElement>\) => \{[\s\S]*?event\.preventDefault\(\);[\s\S]*?event\.stopPropagation\(\);[\s\S]*?void navigateFrontmatterLink\(activeThreadLinkTarget\);[\s\S]*?\}, \[activeThreadLinkTarget\]\);/,
  );
  assert.match(source, /\{activeThreadID && \(/);
  assert.match(
    source,
    /className="op-activity-panel-metadata op-md-frontmatter-properties" aria-label="Thread metadata"[\s\S]*?className="op-activity-panel-metadata-inner"[\s\S]*?className="op-md-frontmatter-property-label"[\s\S]*?<span>Thread<\/span>[\s\S]*?className="op-md-frontmatter-property-value op-md-frontmatter-property-value-link"[\s\S]*?onClick=\{handleActivityThreadIDClick\}/,
  );
  assert.match(
    source,
    /threadMetadataViewModel\.chatFileName && \([\s\S]*?<span>File<\/span>[\s\S]*?className="op-md-frontmatter-property-value op-md-frontmatter-property-value-link"[\s\S]*?onClick=\{handleActivityChatFileClick\}/,
  );
  assert.match(
    source,
    /threadMetadataViewModel\.createdAtLabel && \([\s\S]*?<span>Created<\/span>[\s\S]*?className="op-md-frontmatter-property-value"[\s\S]*?\{threadMetadataViewModel\.createdAtLabel\}/,
  );
  const bodyIndex = source.indexOf('<ThreadActivityView');
  const metadataIndex = source.indexOf('className="op-activity-panel-metadata');
  const loadEarlierIndex = source.indexOf('className="op-activity-panel-window-control is-top"');
  assert.ok(bodyIndex >= 0, 'thread activity body should exist');
  assert.ok(metadataIndex > bodyIndex, 'thread metadata should render inside the activity body');
  assert.ok(loadEarlierIndex < 0 || metadataIndex < loadEarlierIndex, 'thread metadata should be the first body block');
  assert.match(stylesSource, /\.op-activity-panel-metadata\s*\{/);
  assert.match(stylesSource, /\.op-activity-panel-metadata-inner\s*\{/);
  assert.match(stylesSource, /\.op-md-frontmatter-property-row\s*\{/);
  assert.match(stylesSource, /\.op-md-frontmatter-property-value-link\s*\{/);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-metadata-link-btn/);
  assert.doesNotMatch(stylesSource, /\.op-activity-panel-thread-id\s*\{/);
});

test('activity runs keep the user panel state sticky across normal runs', () => {
  assert.doesNotMatch(source, /resetActivityUserOverride/);
  assert.doesNotMatch(source, /Normal completion/);
  assert.doesNotMatch(source, /setExpanded\(activityKey,\s*false/);
  assert.match(source, /shouldKeepActivityPanelExpandedAfterRun\(latestStep, activityErrorInfo\)/);
});

test('expanded activity panel follows the streaming tail without hijacking manual upward scrolls', () => {
  assert.match(source, /const bodyRef = useRef<HTMLDivElement \| null>\(null\);/);
  assert.match(source, /const shouldFollowBottomRef = useRef\(true\);/);
  assert.match(source, /function isActivityPanelNearBottom\(element: HTMLDivElement\): boolean \{/);
  assert.match(source, /useLayoutEffect\(\(\) => \{/);
  assert.match(source, /if \(awaitingUser\) \{\s*return;\s*\}\s*if \(isHistoricalWindow\) \{\s*return;\s*\}\s*const hasLiveStreamingOutput = inProgress \|\| Boolean\(streamingText\);/);
  assert.match(source, /if \(!justExpanded && !shouldFollowBottomRef\.current\) \{/);
  assert.match(source, /body\.scrollTop = body\.scrollHeight;/);
  assert.match(source, /<ThreadActivityView[\s\S]*bodyRef=\{bodyRef\}[\s\S]*bodyMaxHeight=\{bodyMaxHeight\}/);
});

test('awaiting user input forces the activity panel open and reveals questions at the top', () => {
  assert.match(source, /const lastRevealedAwaitingRequestRef = useRef\(''\);/);
  assert.match(source, /const pendingAwaitingRevealRef = useRef\(false\);/);
  assert.match(source, /pendingAwaitingRevealRef\.current = true;\s*setExpanded\(activityKey, true\);/);
  assert.match(source, /body\.scrollTop = 0;/);
  assert.doesNotMatch(source, /!activityKey \|\| !awaitingUser \|\| userOverride === 'collapsed'/);

  const questionIndex = source.indexOf('{awaitingUser && (');
  const planIndex = source.indexOf('{showPlanCard && buildConfig && (');
  assert.ok(questionIndex >= 0, 'question card should render');
  assert.ok(planIndex >= 0, 'plan card should render');
  assert.ok(questionIndex < planIndex, 'question card should be first in the activity body');
});
