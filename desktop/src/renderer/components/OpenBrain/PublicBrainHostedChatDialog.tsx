import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  createOpenBrainPublicBrainConversation,
  listOpenBrainRuntimeModels,
  quoteOpenBrainPublicBrainTurn,
  runOpenBrainPublicBrainBYOKTurn,
  runOpenBrainPublicBrainTurn,
  type OpenBrainPublicBrainFunding,
  type OpenBrainPublicBrainQuote,
} from '../../services/openBrainService';
import { useTabManagerStore } from '../../store/tabManagerStore';

type HostedBrain = {
  brainID: string;
  name: string;
  username: string;
};

type HostedMessage = {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  citations?: Array<{ citationId: string; title: string; excerpt?: string }>;
  funding?: OpenBrainPublicBrainFunding;
  executionMode?: 'hosted' | 'runtime_byok';
  modelKey?: string;
};

type PublicBrainExecutionMode = 'hosted' | 'runtime_byok';

export function PublicBrainHostedChatDialog({ brain, onClose }: { brain: HostedBrain; onClose: () => void }) {
  const workspaceTabIDRef = useRef(useTabManagerStore.getState().activeTabId);
  const [conversationID, setConversationID] = useState('');
  const [conversationMode, setConversationMode] = useState<PublicBrainExecutionMode | null>(null);
  const [mode, setMode] = useState<PublicBrainExecutionMode>('hosted');
  const [question, setQuestion] = useState('');
  const [messages, setMessages] = useState<HostedMessage[]>([]);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const [running, setRunning] = useState(false);
  const [runtimeModels, setRuntimeModels] = useState<Array<{ key: string; name: string; provider: string }>>([]);
  const [modelKey, setModelKey] = useState('');
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const title = useMemo(() => `According to ${brain.name || brain.username}'s public brain`, [brain.name, brain.username]);

  useEffect(() => {
    let active = true;
    void listOpenBrainRuntimeModels(workspaceTabIDRef.current).then((result) => {
      if (!active) return;
      setRuntimeModels(result.models);
      const defaultKey = result.models.some((model) => model.key === result.defaultModelKey)
        ? result.defaultModelKey
        : result.models[0]?.key || '';
      setModelKey(defaultKey);
    }).catch(() => {
      if (active) {
        setRuntimeModels([]);
        setModelKey('');
      }
    });
    return () => { active = false; };
  }, []);

  const ensureConversation = async (executionMode: PublicBrainExecutionMode) => {
    if (conversationID && conversationMode === executionMode) return conversationID;
    const conversation = await createOpenBrainPublicBrainConversation(brain.brainID, executionMode, workspaceTabIDRef.current);
    setConversationID(conversation.conversationId);
    setConversationMode(executionMode);
    return conversation.conversationId;
  };

  const run = async (text: string, quote: OpenBrainPublicBrainQuote, targetConversationID: string) => {
    setRunning(true);
    setError('');
    setStatus(mode === 'runtime_byok' ? 'Retrieving verified evidence, then using your runtime model…' : 'Retrieving cited brain sources…');
    const turnID = `pbt_${crypto.randomUUID()}`;
    setMessages((current) => [...current, { id: `user-${turnID}`, role: 'user', text }]);
    setQuestion('');
    let completed: HostedMessage | null = null;
    try {
      if (mode === 'runtime_byok') {
        if (!modelKey) throw new Error('runtime_model_unavailable');
        const result = await runOpenBrainPublicBrainBYOKTurn(brain.brainID, targetConversationID, {
          turnId: turnID,
          quoteId: quote.quoteId,
          question: text,
          maxAuthorizedDebitMicrousd: quote.maxAuthorizedDebitMicrousd,
          modelKey,
          history: messages.slice(-6).map((message) => ({ role: message.role, text: message.text.slice(0, 1000) })),
        }, workspaceTabIDRef.current);
        completed = {
          id: `assistant-${turnID}`, role: 'assistant', text: result.answer,
          citations: result.citations, funding: result.funding,
          executionMode: 'runtime_byok', modelKey: result.modelKey || modelKey,
        };
      } else {
        await runOpenBrainPublicBrainTurn(
          brain.brainID,
          targetConversationID,
          {
            turnId: turnID,
            quoteId: quote.quoteId,
            question: text,
            maxAuthorizedDebitMicrousd: quote.maxAuthorizedDebitMicrousd,
          },
          (event) => {
            if (event.type === 'retrieving') setStatus('Retrieving cited brain sources…');
            if (event.type === 'synthesizing') setStatus('Generating from the cited evidence…');
            if (event.type === 'error') throw new Error(event.code || 'Public brain turn failed.');
            if (event.type === 'complete') {
              completed = {
                id: `assistant-${turnID}`, role: 'assistant', text: event.answer || '',
                citations: event.citations || [], funding: event.funding, executionMode: 'hosted',
              };
            }
          },
          workspaceTabIDRef.current,
        );
      }
      if (!completed) throw new Error('The public brain turn ended without an answer.');
      setMessages((current) => [...current, completed as HostedMessage]);
      setStatus('');
    } catch (runError) {
      setError(publicBrainErrorMessage(runError));
      setStatus('');
    } finally {
      setRunning(false);
    }
  };

  const prepare = async () => {
    const text = question.trim();
    if (!text || text.length > 1000 || running) return;
    setRunning(true);
    setError('');
    setStatus('Checking access and AI Balance…');
    try {
      if (mode === 'runtime_byok' && !modelKey) throw new Error('runtime_model_unavailable');
      const targetConversationID = await ensureConversation(mode);
      const quote = await quoteOpenBrainPublicBrainTurn(brain.brainID, targetConversationID, text, workspaceTabIDRef.current);
      await run(text, quote, targetConversationID);
    } catch (prepareError) {
      setError(publicBrainErrorMessage(prepareError));
      setStatus('');
      setRunning(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[180] grid place-items-center bg-black/45 p-3" role="dialog" aria-modal="true" aria-labelledby="public-brain-hosted-title">
      <div className="grid h-[min(780px,calc(100vh-24px))] w-[min(820px,calc(100vw-24px))] grid-rows-[auto_1fr_auto] overflow-hidden rounded-2xl border border-border bg-primary-bg shadow-2xl">
        <header className="flex items-center justify-between gap-4 border-b border-border px-5 py-4">
          <div className="min-w-0">
            <span className="font-mono text-[10px] uppercase tracking-[.12em] text-[#2f8f6b]">OpenBrain {mode === 'runtime_byok' ? 'Your model' : 'Hosted'} · @{brain.username}</span>
            <h2 id="public-brain-hosted-title" className="truncate text-base font-semibold text-prime-text">{title}</h2>
          </div>
          <button type="button" onClick={onClose} className="rounded-full border border-border px-3 py-1.5 text-xs text-secondary-text hover:text-prime-text" aria-label="Close public brain chat">Close</button>
        </header>

        <div className="flex min-h-0 flex-col gap-3 overflow-y-auto p-5" aria-live="polite" aria-busy={running}>
          {messages.length === 0 ? (
            <div className="m-auto max-w-lg text-center">
              <h3 className="text-lg font-semibold text-prime-text">Ask this public brain</h3>
              <p className="mt-2 text-sm leading-6 text-secondary-text">Hosted works without a local model. “Your model” retrieves signed, owner-scoped evidence from Cloud and generates in this active runtime; your provider credentials never enter the Cloud Brain API.</p>
            </div>
          ) : messages.map((message) => (
            <article key={message.id} className={`max-w-[84%] rounded-2xl px-4 py-3 ${message.role === 'user' ? 'ml-auto bg-prime-text text-primary-bg' : 'mr-auto border border-border bg-secondary-bg text-prime-text'}`}>
              <span className="mb-1 block font-mono text-[10px] uppercase tracking-wider opacity-60">{message.role === 'user' ? 'You' : 'OpenBrain'}</span>
              <p className="whitespace-pre-wrap text-sm leading-6">{message.text}</p>
              {message.citations?.length ? <div className="mt-3 grid gap-1 border-t border-border pt-2">
                {message.citations.map((citation, index) => <details key={citation.citationId}><summary className="cursor-pointer font-mono text-[11px] text-[#2f8f6b]">[{index + 1}] {citation.title}</summary>{citation.excerpt ? <p className="mt-1 text-xs leading-5 opacity-70">{citation.excerpt}</p> : null}</details>)}
              </div> : null}
              {message.role === 'assistant' ? <div className="mt-3 flex flex-wrap gap-x-3 border-t border-border pt-2 font-mono text-[10px] opacity-60">
                <span>{message.executionMode === 'runtime_byok' ? `Generated by your runtime model${message.modelKey ? ` · ${message.modelKey}` : ''}` : 'AI-generated from cited brain sources'}</span>
                {message.funding?.actualDebitU ? <span>{message.executionMode === 'runtime_byok' ? 'Cloud retrieval' : 'Actual charge'}: {message.funding.actualDebitU} U</span> : null}
              </div> : null}
            </article>
          ))}
          {running ? <div className="font-mono text-xs text-secondary-text">{status || 'Working…'}</div> : null}
        </div>

        <footer className="border-t border-border bg-secondary-bg/50 p-4">
          {error ? <div className="mb-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-prime-text" role="alert">{error}</div> : null}
          <div className="mb-3 flex flex-wrap items-center gap-2 text-xs">
            <div className="inline-flex rounded-full border border-border bg-primary-bg p-1" aria-label="Public brain execution mode">
              <button type="button" disabled={running || messages.length > 0} onClick={() => { setMode('hosted'); setConversationID(''); setConversationMode(null); }} className={`rounded-full px-3 py-1.5 ${mode === 'hosted' ? 'bg-prime-text text-primary-bg' : 'text-secondary-text'}`}>Hosted</button>
              <button type="button" disabled={running || messages.length > 0 || runtimeModels.length === 0} onClick={() => { setMode('runtime_byok'); setConversationID(''); setConversationMode(null); }} className={`rounded-full px-3 py-1.5 ${mode === 'runtime_byok' ? 'bg-prime-text text-primary-bg' : 'text-secondary-text'} disabled:opacity-40`}>Your model</button>
            </div>
            {mode === 'runtime_byok' ? <label className="flex min-w-[220px] flex-1 items-center gap-2 text-secondary-text"><span>Runtime model</span><select value={modelKey} disabled={running} onChange={(event) => setModelKey(event.target.value)} className="min-w-0 flex-1 rounded-lg border border-border bg-primary-bg px-2 py-1.5 text-prime-text"><option value="">Select a configured model</option>{runtimeModels.map((model) => <option key={model.key} value={model.key}>{model.name || model.key}</option>)}</select></label> : null}
            {runtimeModels.length === 0 ? <span className="text-secondary-text">Add a provider model in Models to enable runtime BYOK.</span> : null}
          </div>
          <form onSubmit={(event) => { event.preventDefault(); void prepare(); }} className="rounded-xl border border-border bg-primary-bg focus-within:border-[#2f8f6b]">
            <label htmlFor="desktop-public-brain-question" className="sr-only">Ask this public brain</label>
            <textarea ref={inputRef} id="desktop-public-brain-question" value={question} onChange={(event) => setQuestion(event.target.value.slice(0, 1000))} onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); event.currentTarget.form?.requestSubmit(); } }} rows={2} disabled={running} placeholder={`Ask ${brain.name || brain.username}'s public brain…`} className="w-full resize-none bg-transparent px-4 pt-3 text-sm text-prime-text outline-none" autoFocus />
            <div className="flex items-center justify-between px-3 pb-3 pt-1 font-mono text-[10px] text-secondary-text"><span>{question.length}/1000 · {mode === 'runtime_byok' ? 'Cloud retrieval uses AI Balance; generation uses your provider' : 'Actual model usage uses AI Balance'}</span><button type="submit" disabled={running || !question.trim() || (mode === 'runtime_byok' && !modelKey)} className="rounded-full bg-prime-text px-4 py-2 text-primary-bg disabled:opacity-40">Send</button></div>
          </form>
        </footer>
      </div>
    </div>
  );
}

function publicBrainErrorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error || '');
  if (message.includes('login_required') || message.includes('unauthorized')) return 'Sign in to ask a public brain.';
  if (message.includes('public_brain_disabled')) return 'Public Brain conversation is temporarily unavailable.';
  if (message.includes('ai_balance_required')) return 'AI Balance is too low. Top up in Billing; no Workspace plan is required.';
  if (message.includes('membership_required')) return 'This brain requires an active access membership. AI usage is still charged separately.';
  if (message.includes('brain_not_ready')) return 'This public brain is not currently available.';
  if (message.includes('runtime_byok_not_allowed')) return 'This brain does not allow verified evidence to be sent to your selected model. Use Hosted mode.';
  if (message.includes('runtime_model_unavailable')) return 'The selected model is not available in this active runtime. Check this local or remote tab\'s Models settings.';
  if (message.includes('runtime_provider_failed')) return 'Cloud evidence was retrieved and its retrieval usage remains charged, but your model provider failed. Check its endpoint and credentials, then retry with a new turn.';
  if (message.includes('invalid_evidence')) return 'The signed Cloud evidence could not be verified, so your model was not called.';
  if (message.includes('billing_unavailable')) return 'Funding is temporarily unavailable. No model ran and nothing was charged.';
  return message || 'The public brain turn failed.';
}
