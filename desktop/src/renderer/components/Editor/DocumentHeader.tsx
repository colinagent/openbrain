import React from 'react';

type DocumentHeaderProps = {
  title: string;
  modifiedLabel: string | null;
};

export function DocumentHeader({ title, modifiedLabel }: DocumentHeaderProps) {
  return (
    <header className="op-md-document-header" aria-label={title}>
      <h1 className="op-md-document-header-title">{title}</h1>
      {modifiedLabel ? (
        <p className="op-md-document-header-meta">{modifiedLabel}</p>
      ) : null}
    </header>
  );
}
