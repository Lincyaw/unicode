import type { ChangeKind, ClosedTab, DriverMode, EdgeKind, NodeKind, Status, ViewKind } from './api/types';

/*
 * Every word the pages show, in formal Chinese, after the term table of docs/design.md §7.5. A concept has one word
 * here and every page takes it from here; the words-from-terms lint rule keeps Chinese out of every other module.
 */

export const APP = {
  name: '设计图',
  search: '搜索',
  theme: '切换深色与浅色',
  noResults: '没有匹配的节点',
  dirty: '工作区有未提交的修改',
} as const;

/** The view tabs, in their order. */
export const VIEW_NAMES: Record<ViewKind, string> = {
  architecture: '架构',
  ddd: '领域',
  class: '类',
  sequence: '时序',
  activity: '活动',
  events: '事件',
  calls: '调用',
  property: '性质',
  code: '代码',
  c4: 'C4',
};

/** Why a view tab cannot be chosen, as the server gives it (ClosedTab, docs/design.md §7.5.1); the hint sits on the tab itself. */
export const CLOSED_TABS: Record<ClosedTab, string> = {
  'undeclared-notation': '系统文件没有声明该视图的记法',
  'no-input': '图中没有该视图能接受的输入',
};

/** The word for what a breadcrumb names when the input is the whole system. */
export const VIEW_WORD = '视图';

/** What stands between two names that make one, such as a view's and its input's. */
export const NAME_SEPARATOR = ' · ';

export const STATUS_NAMES: Record<Status, string> = {
  verified: '已验证',
  missing: '缺失',
  stub: '骨架',
  contradicted: '不一致',
};

/** What a node is called, by its kind and, where the kind has several, its form. */
export function kindWord(kind: NodeKind, form: string, layer: string): string {
  if (layer === 'code') {
    return CODE_FORMS[form] ?? CODE_KINDS[kind] ?? form;
  }
  return DESIGN_FORMS[form] ?? DESIGN_KINDS[kind];
}

const DESIGN_KINDS: Record<NodeKind, string> = {
  container: '组件',
  type: '类型',
  op: '操作',
  actor: '参与者',
  flow: '流程',
  step: '步骤',
  property: '性质',
  observation: '观察',
  case: '用例',
  run: '运行',
  result: '结果',
  review: '评审',
};

const DESIGN_FORMS: Record<string, string> = {
  system: '系统',
  component: '组件',
  person: '参与者',
  external: '外部系统',
  error: '错误',
  event: '事件',
};

const CODE_KINDS: Partial<Record<NodeKind, string>> = {
  container: '包',
  type: '类型',
  op: '函数',
};

const CODE_FORMS: Record<string, string> = {
  package: '包',
  file: '文件',
};

/*
 * The forms a reader tells apart within a kind, by the word the term table gives each (docs/model.md §3). A form whose
 * word kindWord already says, such as an error type's, maps to null.
 */

/** The forms of a step: what it does in its flow. */
const STEP_FORMS: Record<string, string | null> = {
  call: '调用',
  send: '发送',
  emit: '发布',
  async: '异步',
  alt: '分支',
  opt: '可选',
  loop: '循环',
  on: '错误处理',
};

/** The forms of a property (docs/model.md §3.3): where its claim holds. */
const PROPERTY_FORMS: Record<string, string | null> = {
  invariant: '不变式',
  postcondition: '后置条件',
  relation: '关系',
  transition: '状态转移',
};

/** The forms of a design type; an error's and an event's are their kind words. */
const DESIGN_TYPE_FORMS: Record<string, string | null> = {
  object: '对象',
  record: '记录',
  interface: '接口',
  enum: '枚举',
  error: null,
  event: null,
};

/** The forms of a code function; a plain function's is its kind word. */
const CODE_OP_FORMS: Record<string, string | null> = {
  func: null,
  method: '方法',
  abstract: '抽象方法',
};

/** The forms of an observation. */
const OBSERVATION_FORMS: Record<string, string | null> = {
  snapshot: null,
};

/**
 * The forms of a node's kind that a reader tells apart and the kind's word does not say; undefined for a kind whose
 * word says its form (a container, an actor, a flow, a design operation) and for a code type, whose forms are each
 * language's own.
 */
function formsOf(node: { kind: NodeKind; layer: string }): Record<string, string | null> | undefined {
  switch (node.kind) {
    case 'step':
      return STEP_FORMS;
    case 'property':
      return PROPERTY_FORMS;
    case 'case':
      return { declared: CASE_SOURCES.declared, recorded: CASE_SOURCES.recorded };
    case 'observation':
      return OBSERVATION_FORMS;
    case 'type':
      return node.layer === 'design' ? DESIGN_TYPE_FORMS : undefined;
    case 'op':
      return node.layer === 'code' ? CODE_OP_FORMS : undefined;
    case 'container':
    case 'actor':
    case 'flow':
    case 'run':
    case 'result':
    case 'review':
      return undefined;
  }
}

/** The word for a node's form where kindWord does not say it; a form of a closed set that the term table lacks is an error. */
export function formWord(node: { kind: NodeKind; form: string; layer: string }): string | undefined {
  const forms = formsOf(node);
  if (forms === undefined) {
    return undefined;
  }
  const word = forms[node.form];
  if (word === undefined) {
    throw new Error(`The term table has no word for the ${node.layer} ${node.kind} form ${node.form} (docs/design.md §7.5)`);
  }
  return word ?? undefined;
}

/** What an edge is called, by its kind (docs/model.md §4). */
export const EDGE_KINDS: Record<EdgeKind, string> = {
  contains: '包含',
  uses: '声明的依赖',
  extends: '继承',
  implements: '实现接口',
  refers: '引用',
  subscribes: '订阅',
  imports: '导入',
  calls: '调用',
  from: '发起',
  emits: '发布',
  handles: '处理',
  about: '约束',
  realizes: '实现',
  checks: '检查',
  generates: '生成',
  observes: '观察',
  substitutes: '替身',
  publishes: '发布边界',
  owns: '归属',
  depends: '依赖',
  traces: '代码追溯',
  exercises: '可达',
  reaches: '经由细节函数的调用',
  covers: '覆盖',
};

/** The attributes a node's change names (docs/design.md §5.2), by their keys. */
const ATTRIBUTE_NAMES: Record<string, string> = {
  focus: '关注级别',
  labels: '标签',
  paths: '路径',
  notations: '记法',
  roots: '根目录',
  language: '语言',
  path: '路径',
  fields: '字段',
  values: '取值',
  signature: '签名',
  pure: '纯操作',
  codec: '编解码操作',
  codecType: '可回放表示类型',
  symbols: '符号',
  limits: '限定',
  call: '被测调用',
  within: '时限',
  ensure: '断言',
  manual: '人工性质的原因',
  requires: '前提',
  observe: '观察范围',
  changes: '允许变化的范围',
  type: '观察类型',
  inputs: '输入',
  index: '序号',
  label: '分支标签',
  args: '实参',
  reason: '录制原因',
  name: '名称',
  declaration: '声明',
  testKind: '测试类别',
  stub: '骨架',
  body: '函数体',
  generated: '生成',
  globals: '顶层声明',
};

/** The word for an attribute's key; a key the term table lacks is an error. */
export function attributeName(key: string): string {
  const name = ATTRIBUTE_NAMES[key];
  if (name === undefined) {
    throw new Error(`The term table has no word for the attribute ${key} (docs/design.md §5.2, §7.5)`);
  }
  return name;
}

export const SECTIONS = {
  attributes: '属性',
  focus: '关注级别',
  labels: '标签',
  paths: '路径',
  container: '组件',
  dependsOn: '依赖',
  dependedOnBy: '被依赖',
  contents: '包含',
  code: '代码',
  views: '相关视图',
  signature: '签名',
  signatureTypes: '签名中的类型',
  calls: '调用',
  caller: '调用方',
  callee: '被调用方',
  args: '实参',
  trace: '代码追溯',
  realizedBy: '实现',
  callSites: '调用位置',
  children: '子步骤',
  stepCases: '覆盖本步骤的用例',
  coveredBy: '覆盖到它的用例',
  fields: '字段',
  values: '枚举值',
  ops: '操作',
  refers: '引用',
  referredBy: '被引用',
  relations: '关联',
  supertypes: '继承与实现接口',
  properties: '相关性质',
  flows: '参与的流程',
  emittedBy: '发布',
  subscribers: '订阅',
  raisedBy: '抛出',
  handledBy: '处理',
  handles: '处理的错误',
  call: '所在调用',
  via: '经过的细节函数',
  end: '终点',
  steps: '相关步骤',
  expand: '展开',
  collapse: '收起',
  valuesOfCase: '取值',
  history: '历史',
  covers: '覆盖的函数',
  location: '位置',
  file: '文件',
  line: '行',
  details: '细节函数',
  about: '约束对象',
  inputs: '输入',
  predicate: '谓词',
  generators: '生成器',
  cases: '用例',
  adequacy: '测试充分性',
  trail: '导航路径',
  changes: '变更',
  review: '评审',
  diagnostics: '诊断',
  fix: '修正',
  owner: '所属组件',
  stepsOfFlow: '步骤',
  imports: '导入与调用',
} as const;

export const EVENTS_COLUMNS = {
  emitters: '发布',
  events: '事件',
  subscribers: '订阅',
  raisers: '抛出',
  errors: '错误',
  handlers: '处理',
} as const;

export const CASE_COLUMNS = {
  name: '名称',
  source: '来源',
  values: '取值',
  outcome: '结果',
  gain: '覆盖增量',
} as const;

export const MEMBER_COLUMNS = {
  name: '名称',
  type: '类型',
  code: '代码',
  status: '状态',
} as const;

export const OP_COLUMNS = {
  signature: '签名',
  realizedBy: '实现',
  properties: '相关性质',
} as const;

/** Where a case comes from: declared in the design, or recorded for coverage or as a counterexample. */
export const CASE_SOURCES = {
  declared: '声明',
  recorded: '录制',
  counterexample: '反例',
} as const;

export const OUTCOMES = {
  passed: '通过',
  failed: '失败',
} as const;

export const DRIVER_MODES: Record<DriverMode, string> = {
  replay: '回放',
  explore: '探索',
  fuzz: '模糊测试',
};

export const ADEQUACY = {
  code: '代码覆盖率',
  branch: '分支覆盖率',
  error: '错误覆盖率',
} as const;

/** What a case's result in one run was, read along its history. */
export const HISTORY = {
  firstFailure: '首次失败',
  fixed: '修复',
  replayPassed: '回放通过',
  passed: '通过',
  failed: '失败',
} as const;

export const CHANGE_KINDS: Record<ChangeKind, string> = {
  added: '新增',
  removed: '删除',
  modified: '修改',
  unchanged: '不变',
};

/** How a node changed since the base, said from its attribute changes and its code. */
export const CHANGE_WORDS = {
  addedField: (name: string) => `新增字段 ${name}`,
  removedField: (name: string) => `删除字段 ${name}`,
  addedValue: (name: string) => `新增枚举值 ${name}`,
  removedValue: (name: string) => `删除枚举值 ${name}`,
  signatureChanged: '签名修改',
  signatureKept: '签名不变',
  bodyChanged: (lines: number) => `函数体修改 ${lines} 行`,
  attrChanged: (attribute: string) => `${attribute}修改`,
  added: '新增',
  removed: '删除',
  unchanged: '不变',
  separator: '，',
} as const;

export const REVIEW_DECISIONS = {
  approved: '批准',
  'changes-requested': '要求修改',
} as const;

export const REVIEW = {
  pending: '未评审',
  reviewed: '已评审',
} as const;

export const DIAGNOSTIC_WORDS = {
  undeclared: '未声明',
  count: (n: number) => `${n}`,
} as const;

export const CLASS_LAYERS = {
  design: '设计层',
  code: '代码层',
} as const;

export const C4_LEVELS = {
  context: '系统上下文',
  container: '容器',
} as const;

export const CONTROLS = {
  zoomIn: '放大',
  zoomOut: '缩小',
  fit: '适合窗口',
  open: '进入实体页',
  compare: '比较',
  compareNone: '不比较',
  comparedWith: (base: string) => `与 ${base} 比较`,
  diagnostics: '诊断',
  diagnosticsCount: (count: number) => `诊断 ${count} 条`,
  expandRow: '展开',
  collapseRow: '收起',
  viewSwitch: '视图',
  tree: '设计树',
  details: '详情',
  resizeTree: '调整设计树的宽度',
  resizePane: '调整详情栏的宽度',
  layer: '层',
  level: '层级',
  linesUnit: '行',
  colon: '：',
  none: '—',
} as const;

/** What the marks a ⓘ explains are called, so that its name for screen readers says what it explains. */
export const MARKS = {
  status: '状态',
  hidden: '未画出的组件',
  merged: '合并的边',
} as const;

/** The explanations behind the ⓘ marks; the pages show data only (CLAUDE.md principle 10). */
export const ABOUT = {
  tree: '设计层的节点按包含关系组成的树：系统，各组件按关注级别排列，然后是外部系统、参与者与工具组件。圆点是声明项与代码比较的状态。',
  statusLegend: '圆点表示声明项与代码比较的状态：绿色为已验证，黄色为缺失，空心为骨架，红色为不一致。',
  hidden: '未画出的工具与基础服务组件的个数，以及指向它们的依赖。',
  undeclared: '实际存在、但设计中没有声明的依赖；计数是其中的导入与调用。',
  merged: '指向折叠组件的边合并为一条，计数是它代表的边数。',
  reachesCount: '经过的细节函数的个数。',
  card: (node: { kind: NodeKind; form: string; layer: string }, place: string) => [kindWord(node.kind, node.form, node.layer), formWord(node), place].filter(part => part !== undefined).join(' · '),
  trail: '从视图进入实体页时经过的节点，依次排列。',
  stepCases: '覆盖本步骤的用例个数，以及按分支或错误计的个数。',
  coveredBy: '覆盖到它的用例：录制的用例和最近一次运行中执行到它的用例。',
  inputs: '性质对符号的每一组取值都成立；限定缩小取值的范围。',
  predicate: '检查这条性质的谓词，以及调用它的驱动：回放录制的用例、用生成器探索、模糊测试。',
  cases: '声明的用例写在设计文件中，录制的用例由探索按覆盖选出；覆盖增量是相对其他用例新增的代码基本块数。',
  adequacy: '代码覆盖率按测量范围内的 Go 基本块计数，源码位置标出块的范围；分支覆盖率按明确匹配的流程分支计数；错误覆盖率按实际抛出的声明错误计数。未测量的指标不显示。',
  history: '该用例在各次运行中的结果，最新的在下。',
  covers: '该用例执行到的函数及其所在的行。',
  details: '未画出的细节函数：它们不实现设计节点，也没有检查或生成的绑定。',
  fields: '设计中写出的字段与实现它的代码类型逐项比较；代码中多出的字段算细节，不列出。',
  calls: '调用方与被调用方；只经过细节函数的调用画成虚线，标出经过的细节函数个数。',
  code: '实现它的代码元素的源码。',
  diagnostics: '检查快照得到的诊断；每一条都是错误。',
  compare: '与所选提交的快照比较，列出新增、删除和修改的节点与边。',
  architecture: '核心组件展开，显示其中的类型、流程、性质与错误；支撑组件折叠；工具组件不画出。边是实际的依赖，红色虚线为设计中没有声明的依赖。',
  ddd: '带限界上下文标签的组件，其中带领域标签的类型，以及它们之间的引用、继承与实现接口。',
  class: '类型及其字段与操作；实线来自字段，虚线来自操作签名；其他组件中被引用的类型画在折叠的组件中。',
  sequence: '生命线按首次出现排列；每一步标出序号与状态，片段画成框。',
  activity: '同一个流程按控制结构画出：调用嵌套成框，错误处理画成分支，异步片段单独画出。',
  events: '事件、发布它的步骤和订阅它的操作；错误、抛出它的调用步骤和处理它的片段。',
  callsView: '显著函数之间的调用；只经过细节函数的调用画成虚线并标出经过的个数；其他组件的函数合并到折叠的组件中。',
  property: '性质的陈述、约束对象、输入、谓词、用例与测试充分性。',
  codeView: '文件中的类型与函数及其绑定，进出的调用，以及覆盖到它的用例。',
  c4: '系统上下文画出人、系统与外部系统；容器层画出带容器标签的组件；边标出经过它调用的操作。',
} as const;

export const PROBLEM_WORDS = {
  title: '请求被拒绝',
  failed: '页面未能绘制',
  loading: '正在载入',
} as const;
