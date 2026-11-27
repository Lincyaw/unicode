import { useComparison, useDiagnostics, useRevisions } from '../../api/client';
import { CompareButton } from '../../components/shell/CompareButton';
import { DiagnosticsButton } from '../../components/shell/DiagnosticsButton';

/** The header's diagnostic count and compare button, after the view tabs. */
export function HeaderTools({ base, baseHref, subjectHref, entityHref }: {
  base?: string;
  baseHref: (base: string | undefined) => string;
  subjectHref: (subject: string) => string | undefined;
  entityHref: (id: string) => string;
}) {
  const { diagnostics } = useDiagnostics();
  const revisions = useRevisions().data ?? [];
  const comparison = useComparison(base).data;
  return (
    <>
      <DiagnosticsButton diagnostics={diagnostics} subjectHref={subjectHref} />
      <CompareButton base={base} revisions={revisions} comparison={comparison} baseHref={baseHref} entityHref={entityHref} />
    </>
  );
}
