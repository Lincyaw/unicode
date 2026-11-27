import type { Problem } from './types';

/** A refused request: the interface's Problem body (code, cause, fix), and the HTTP status it came with. */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem;

  constructor(status: number, problem: Problem) {
    super(`${problem.code}: ${problem.message}`);
    this.status = status;
    this.problem = problem;
  }
}
