import { useRouteError } from 'react-router';
import { ProblemNote } from '../components/details/ProblemNote';

/** A page whose address the viewer cannot read, or that fails while it draws: the problem, where the page would be. */
export function AddressProblem() {
  return <ProblemNote error={useRouteError()} />;
}
