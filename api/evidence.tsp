namespace Unicode.Viewer;

/** How a check ran: replaying cases, exploring with generators, or fuzzing (docs/design.md §6.2–§6.3). */
enum DriverMode {
  replay,
  explore,
  fuzz,
}

/** The result of a check on an input. */
enum Outcome {
  passed,
  failed,
}

/** M4: one generated driver of a predicate, such as 回放 · 4 or 探索 · @given. */
model Driver {
  mode: DriverMode;

  /** The library that drives it, such as rapid, @given or atheris */
  tool: string;

  /** Replay: how many cases it replays */
  cases?: int32;
}

/** M6: 测试充分性, each a ratio from 0 to 1; absent when not measured. */
model Adequacy {
  /** 代码覆盖率: the lines executed among the functions the predicate exercises */
  code?: float64;

  /** 分支覆盖率: the steps and branches of the flows reached */
  branch?: float64;

  /** 错误覆盖率: the declared errors actually raised */
  error?: float64;
}

/** M6: one result of a case in one run. */
model RunResult {
  /** The commit the run checked */
  commit: string;

  at: utcDateTime;
  mode: DriverMode;
  outcome: Outcome;
}

/** M6: how many of the cases that cover a step or a function took one branch or raised one error. */
model BranchCount {
  /** The branch label or the error, such as Declined */
  label: string;

  cases: int32;
}

/** M6 (drivers M4): what runs have shown about a node. Every field is absent until the graph holds evidence. */
model Evidence {
  /** A case, a property or a flow: its latest outcome */
  outcome?: Outcome;

  /** A case: 覆盖增量, the lines it adds to what its predicate's other cases cover */
  coverageGain?: int32;

  /** A property or a flow */
  adequacy?: Adequacy;

  /** A property or a flow: its generated drivers */
  drivers?: Driver[];

  /** A case: 历史, its results, newest first */
  history?: RunResult[];

  /** A step or a function: the covering cases by branch */
  branches?: BranchCount[];
}

/** M7: 评审：未评审、已评审. */
enum ReviewState {
  /** Never reviewed, or reviewed at another content hash (docs/model.md §8, 待审) */
  pending,

  reviewed,
}

/** M7: the review record of a node. */
model Review {
  state: ReviewState;
  reviewer?: string;
  at?: utcDateTime;

  /** The content hash it was reviewed at */
  digest?: string;

  comment?: string;
}
