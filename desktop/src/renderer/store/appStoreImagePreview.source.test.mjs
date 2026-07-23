import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const appStoreSourcePath = new URL('./appStore.ts', import.meta.url);
const appStoreSource = await readFile(appStoreSourcePath, 'utf8');
const editorRegistrySourcePath = new URL('../services/editorRegistry.ts', import.meta.url);
const editorRegistrySource = await readFile(editorRegistrySourcePath, 'utf8');
const appSourcePath = new URL('../App.tsx', import.meta.url);
const appSource = await readFile(appSourcePath, 'utf8');
const imageEditorSourcePath = new URL('../components/Editor/ImageEditor.tsx', import.meta.url);
const imageEditorSource = await readFile(imageEditorSourcePath, 'utf8');
const bookReaderSourcePath = new URL('../components/Editor/BookReaderEditor.tsx', import.meta.url);
const bookReaderSource = await readFile(bookReaderSourcePath, 'utf8');
const epubBookViewSourcePath = new URL('../components/Editor/EpubBookView.tsx', import.meta.url);
const epubBookViewSource = await readFile(epubBookViewSourcePath, 'utf8');
const pdfBookViewSourcePath = new URL('../components/Editor/PdfBookView.tsx', import.meta.url);
const pdfBookViewSource = await readFile(pdfBookViewSourcePath, 'utf8');
const bookNoteWidgetSourcePath = new URL('../components/Editor/codemirror/widgets/BookNoteWidget.ts', import.meta.url);
const bookNoteWidgetSource = await readFile(bookNoteWidgetSourcePath, 'utf8');
const livePreviewBlockDecorationsSourcePath = new URL('../components/Editor/codemirror/livePreviewBlockDecorations.ts', import.meta.url);
const livePreviewBlockDecorationsSource = await readFile(livePreviewBlockDecorationsSourcePath, 'utf8');
const fileTreeItemSourcePath = new URL('../components/FileExplorer/FileTreeItem.tsx', import.meta.url);
const fileTreeItemSource = await readFile(fileTreeItemSourcePath, 'utf8');
const indexHtmlPath = new URL('../../../index.html', import.meta.url);
const indexHtmlSource = await readFile(indexHtmlPath, 'utf8');

test('image extensions resolve to the image editor', () => {
  assert.match(editorRegistrySource, /const IMAGE_EXTENSIONS = new Set\(\[[^\]]+\]\)/s);
  for (const extension of ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.svg']) {
    assert.match(editorRegistrySource, new RegExp(`'\\${extension}'`));
  }
  assert.match(editorRegistrySource, /if \(IMAGE_EXTENSIONS\.has\(extension\)\) \{\s*return 'image';\s*\}/s);
});

test('book extensions resolve to the book reader', () => {
  assert.match(editorRegistrySource, /const BOOK_EXTENSIONS = new Set\(\[[^\]]*'\.epub'[^\]]*'\.pdf'[^\]]*\]\)/s);
  assert.match(editorRegistrySource, /if \(BOOK_EXTENSIONS\.has\(extension\)\) \{\s*return 'book';\s*\}/s);
});

test('openFile creates binary preview tabs without reading binary content as text', () => {
  assert.match(appStoreSource, /resourceVersion\?: number/);
  assert.match(appStoreSource, /const isBinaryPreview =\s*editorId === 'image' \|\| editorId === 'pdf' \|\| editorId === 'book';/);
  assert.match(appStoreSource, /if \(!isBinaryPreview\) \{\s*const result = await fileService\.readFile\(path\)/s);
  assert.match(appStoreSource, /resourceVersion: isBinaryPreview \? 0 : undefined/);
});

test('binary preview tab reloads use stat and resource version instead of readFile', () => {
  assert.match(appStoreSource, /function isBinaryPreviewDocumentTab/);
  assert.match(appStoreSource, /filter\(isBinaryPreviewDocumentTab\)/);
  assert.match(appStoreSource, /result: await fileService\.stat\(path\)/);
  assert.match(appStoreSource, /resourceVersion: \(tab\.resourceVersion \?\? 0\) \+ 1/);
});

test('App registers and renders binary preview editors', () => {
  assert.match(appSource, /import \{ ImageEditor \} from '\.\/components\/Editor\/ImageEditor'/);
  assert.match(appSource, /import \{ BookReaderEditor \} from '\.\/components\/Editor\/BookReaderEditor'/);
  assert.match(appSource, /id: 'image',\s*displayName: 'Image Preview',\s*component: ImageEditor/s);
  assert.match(appSource, /id: 'pdf',\s*displayName: 'Book Reader',\s*component: BookReaderEditor/s);
  assert.match(appSource, /id: 'book',\s*displayName: 'Book Reader',\s*component: BookReaderEditor/s);
  assert.match(appSource, /if \(tab\.editorId === 'image'\) \{\s*return <ImageEditor tabId=\{isPinned \? tab\.id : undefined\} \/>;\s*\}/s);
  assert.match(appSource, /if \(tab\.editorId === 'book' \|\| tab\.editorId === 'pdf'\) \{\s*return <BookReaderEditor tabId=\{isPinned \? tab\.id : undefined\} \/>;\s*\}/s);
});

test('ImageEditor renders through the resource service', () => {
  assert.match(imageEditorSource, /getRenderUrlForPhysicalPath\(filePath\)/);
  assert.doesNotMatch(imageEditorSource, /readFile\(/);
  assert.match(imageEditorSource, /writeClipboardImageFromElement\(imageRef\.current\)/);
});

test('BookReaderEditor renders through the resource service and writes notes through workspace file actions', () => {
  assert.match(bookReaderSource, /getRenderHandleForPhysicalPath\(filePath\)/);
  assert.match(bookReaderSource, /resolveEpubOpen\(resourceMeta, renderHandle, renderUrl\)/);
  assert.match(bookReaderSource, /readTextFile\(notePath\)/);
  assert.match(bookReaderSource, /writeTextFile\(notePath, content\)/);
  assert.match(bookReaderSource, /removeBookHighlightNote\(existing, target\)/);
  assert.match(bookReaderSource, /parseBookHighlightNotes\(content\)/);
  assert.match(bookReaderSource, /filterBookHighlightNotesForTarget/);
  assert.match(bookReaderSource, /cfi: isEpubSelection\(targetSelection\) \? targetSelection\.cfiRange : null/);
  assert.match(bookReaderSource, /rects: isPdfSelection\(targetSelection\) \? targetSelection\.rects : null/);
  assert.match(bookReaderSource, /onHighlightContextAction=\{handleHighlightContextAction\}/);
  assert.doesNotMatch(bookReaderSource, /<iframe/);
});

test('book reader uses the bottom app status bar and a compact action menu', () => {
  assert.match(appSource, /const activeStatusFilePath = \(activeDocument\?\.filePath \|\| currentFilePath \|\| ''\)\.trim\(\);/);
  assert.match(appSource, /title=\{statusBarPathDisplay\.label\}[\s\S]*?\{statusBarPathDisplay\.label\}/);
  assert.match(bookReaderSource, /MoreHorizontalIcon/);
  assert.match(bookReaderSource, /PopupMenu className="absolute right-0 top-8 w-64 text-secondary-text"/);
  assert.match(bookReaderSource, /aria-label="Reader actions"/);
  assert.match(bookReaderSource, /className="fixed inset-0 z-20"/);
  assert.match(bookReaderSource, /onPointerDown=\{\(\) => setReaderMenuOpen\(false\)\}/);
  assert.match(bookReaderSource, /Save highlight/);
  assert.match(bookReaderSource, /Open notes/);
  assert.doesNotMatch(bookReaderSource, /title=\{filePath \|\| tab\?\.title \|\| 'Book'\}/);
  assert.doesNotMatch(bookReaderSource, /Highlight\s*<\/button>/);
});

test('book highlight note blocks can reopen books at saved locations', () => {
  assert.match(appStoreSource, /export type BookOpenTarget/);
  assert.match(appStoreSource, /bookTarget\?: BookOpenTarget/);
  assert.match(appStoreSource, /pendingBookTarget: options\?\.bookTarget \|\| null/);
  assert.match(bookReaderSource, /const pendingBookTarget = tab\?\.pendingBookTarget \|\| null/);
  assert.match(bookReaderSource, /setEpubHighlightRequest/);
  assert.match(bookReaderSource, /setPdfHighlightRequest/);
  assert.match(epubBookViewSource, /highlightRequest\?: EpubHighlightRequest \| null/);
  assert.match(epubBookViewSource, /highlightNotes\?: ParsedBookHighlightNote\[\]/);
  assert.match(epubBookViewSource, /rendition\.display\(cfi\)/);
  assert.match(epubBookViewSource, /appliedHighlightCfisRef/);
  assert.match(pdfBookViewSource, /highlightRequest\?: PdfHighlightRequest \| null/);
  assert.match(pdfBookViewSource, /highlightNotes\?: ParsedBookHighlightNote\[\]/);
  assert.match(pdfBookViewSource, /note\.page === page/);
  assert.match(bookNoteWidgetSource, /openFile\(resolved, \{ bookTarget \}\)/);
  assert.match(livePreviewBlockDecorationsSource, /parseBookHighlightNoteBlock\(code\)/);
  assert.match(livePreviewBlockDecorationsSource, /new BookNoteWidget\(note, currentFilePath, block\.from, block\.to\)/);
});

test('unpacked epub directories behave as book packages in the file tree', () => {
  assert.match(fileTreeItemSource, /function isEpubPackagePath\(path: string, isDir: boolean\)/);
  assert.match(fileTreeItemSource, /endsWith\('\.epub'\)/);
  assert.match(fileTreeItemSource, /isPackage: isEpubPackage/);
  assert.match(fileTreeItemSource, /if \(entry\.isDir && !isEpubPackage\) \{/);
  assert.match(fileTreeItemSource, /entry\.isDir && !isEpubPackage && isExpanded/);
  assert.match(fileTreeItemSource, /isEpubPackage \? <FileTreeBookIcon \/>/);
});

test('EpubBookView uses explicit epubjs open modes and reader state hooks', () => {
  assert.match(epubBookViewSource, /openAs: openMode/);
  assert.match(epubBookViewSource, /replacements: openMode === 'epub' \? 'blobUrl' : 'none'/);
  assert.match(epubBookViewSource, /book\.loaded\.navigation/);
  assert.match(epubBookViewSource, /display\(initialCfi \|\| undefined\)/);
  assert.match(epubBookViewSource, /clearRenditionSelections\(rendition\)/);
  assert.match(epubBookViewSource, /rendition\?\.display\(target\)\.then\(\(\) => scheduleAnnotationRefresh\(rendition\)\)/);
  assert.match(epubBookViewSource, /renderAnnotationPanes\(rendition\)/);
  assert.match(epubBookViewSource, /':target, \*:target'/);
});

test('EpubBookView supports keyboard and trackpad page turns inside epub iframes', () => {
  assert.match(epubBookViewSource, /rendition\.on\('keydown', handlePageKeyDown\)/);
  assert.match(epubBookViewSource, /rendition\.hooks\.content\.register\(attachContentInteractions\)/);
  assert.match(epubBookViewSource, /doc\.addEventListener\('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS\)/);
  assert.match(epubBookViewSource, /root\.addEventListener\('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS\)/);
  assert.match(epubBookViewSource, /event\.key === 'ArrowUp'/);
  assert.match(epubBookViewSource, /event\.key === 'ArrowDown'/);
});

test('content security policy allows local resource iframe previews', () => {
  assert.match(indexHtmlSource, /frame-src[^"]*http:\/\/127\.0\.0\.1:\*/);
  assert.match(indexHtmlSource, /frame-src[^"]*http:\/\/localhost:\*/);
  assert.match(indexHtmlSource, /frame-src[^"]*blob:/);
  assert.match(indexHtmlSource, /img-src[^"]*blob:/);
  assert.match(indexHtmlSource, /font-src[^"]*blob:/);
});
