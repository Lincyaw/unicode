import { Suspense, type ReactNode } from 'react';
import { ErrorBoundary } from 'react-error-boundary';
import { ProblemNote } from '../components/details/ProblemNote';
import { Loading } from '../ui/content';

/**
 * Where a part of the page loads on its own: it waits with a loading note, and a refused request shows its problem in
 * place. `reset` is the address the part shows, so a new address tries again.
 */
export function Boundary({ reset, quiet = false, children }: { reset: string; quiet?: boolean; children: ReactNode }) {
  return (
    <ErrorBoundary FallbackComponent={ProblemNote} resetKeys={[reset]}>
      <Suspense fallback={quiet ? null : <Loading />}>{children}</Suspense>
    </ErrorBoundary>
  );
}
