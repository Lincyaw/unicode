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
  /** 代码覆盖率: the Go basic blocks executed among the functions the predicate exercises */
  code?: float64;

  /** 分支覆盖率: the steps and branches of the flows reached */
  branch?: float64;

  /** 错误覆盖率: the declared errors actually raised */
  error?: float64;
}

/** M8: where a check ran (docs/model.md §3.5): in a test, or in a monitored program on real calls. */
enum Venue {
  test,
  runtime,
}

/** M6: one result of a case in one run. */
model RunResult {
  /** The commit the run checked */
  commit: string;

  at: utcDateTime;
  mode: DriverMode;
  outcome: Outcome;

  /** M8: where it ran */
  venue: Venue;
}

/** M8: a limit or precondition that real calls broke, such as where#1 or requires#1, and on how many calls. */
model ClauseCalls {
  clause: string;
  calls: int32;
}

/** M8: a clause of the statement, such as ensure#2, and how many real calls judged it. */
model ClauseJudged {
  clause: string;
  judged: int32;
}

/** M8: a model value in the canonical encoding of docs/design.md §6.4; a value of a sensitive type is redacted. */
model ModelValue {
  /** The kind, such as int, string, record, list or redacted; it decides whether text, items or entries hold it */
  kind: string;

  text: string;
  items: ModelValue[] | null;
  entries: ModelValueEntry[] | null;
}

/** M8: one entry of a record, a map or a call value. */
model ModelValueEntry {
  key: ModelValue;
  value: ModelValue;
}

/** M8: a real call that contradicts a property or flow. */
model RuntimeCounterexample {
  /** The hash of its input, as frozen and redacted */
  inputHash: string;

  /** The clause that does not hold: ensure#n, or changes, raises or call */
  clause: string;

  /** Its input: each symbol's model value, redacted where its type is sensitive */
  values: Record<ModelValue>;

  /** When the call began */
  at: utcDateTime;
}

/** M8: the current runtime evidence of a property or flow: one record period of a monitored program. */
model RuntimeEvidence {
  /** The commit the monitored program was built from */
  commit: string;

  /** The record period */
  from: utcDateTime;

  to: utcDateTime;

  /** 适用的调用: the real calls whose input the property applies to and that were judged */
  applicable: int32;

  /** 域外调用: by the limit or precondition that did not hold */
  outsideDomain: ClauseCalls[];

  /** 判定次数: how many times each clause was judged */
  clauses: ClauseJudged[];

  counterexamples: RuntimeCounterexample[];

  /** 第 95 百分位耗时, in nanoseconds: only of a property or flow with within */
  p95?: int64;

  /** 核对的轨迹: only of a flow */
  traces?: int32;
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

  /** A case: 覆盖增量, the measured coverage points it adds beyond its predicate's other cases */
  coverageGain?: int32;

  /** A property or a flow */
  adequacy?: Adequacy;

  /** A property or a flow: its generated drivers */
  drivers?: Driver[];

  /** A case: 历史, its results, newest first */
  history?: RunResult[];

  /** A step or a function: the covering cases by branch */
  branches?: BranchCount[];

  /** M8, a property or a flow: its current runtime evidence */
  runtime?: RuntimeEvidence;
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
  target?: ReviewTarget;
  recordId?: string;
  snapshot?: string;
  decision?: "approved" | "changes-requested";
  reviewer?: string;
  at?: utcDateTime;

  /** The content hash it was reviewed at */
  digest?: string;

  comment?: string;
}
