import type { ChangeKind, DriverMode, NodeKind, Status, ViewKind } from './api/types';

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

/** The word for what a breadcrumb names when the input is the whole system. */
export const VIEW_WORD = '视图';

export const STATUS_NAMES: Record<Status, string> = {
  verified: '已验证',
  missing: '缺失',
  stub: '未实现',
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
  supertypes: '继承与实现',
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
  actions: '操作',
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
  imports: '导入',
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
  attrChanged: (key: string) => `${key} 修改`,
  added: '新增',
  removed: '删除',
  unchanged: '不变',
  separator: '，',
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
  diagnostics: '诊断',
  expandRow: '展开',
  collapseRow: '收起',
  viewSwitch: '视图',
  tree: '设计树',
  details: '详情',
  layer: '层',
  level: '层级',
  linesUnit: '行',
  colon: '：',
  none: '—',
} as const;

/** The explanations behind the ⓘ marks; the pages show data only (CLAUDE.md principle 10). */
export const ABOUT = {
  tree: '设计层的节点按包含关系组成的树：系统，各组件按关注级别排列，然后是外部系统、参与者与工具组件。圆点是声明项与代码比较的状态。',
  statusLegend: '圆点表示声明项与代码比较的状态：绿色为已验证，黄色为缺失，空心为未实现，红色为不一致。',
  hidden: '未画出的工具与基础服务组件的个数，以及指向它们的依赖。',
  undeclared: '实际存在、但设计中没有声明的依赖；计数是其中的导入与调用。',
  merged: '指向折叠组件的边合并为一条，计数是它代表的边数。',
  reachesCount: '经过的细节函数的个数。',
  card: (kind: string, form: string, path: string) => `${kind} · ${form} · ${path}`,
  trail: '从视图进入实体页时经过的节点，依次排列。',
  stepCases: '覆盖本步骤的用例个数，以及按分支或错误计的个数。',
  coveredBy: '覆盖到它的用例：录制的用例和最近一次运行中执行到它的用例。',
  inputs: '性质对符号的每一组取值都成立；限定缩小取值的范围。',
  predicate: '检查这条性质的谓词，以及调用它的驱动：回放录制的用例、用生成器探索、模糊测试。',
  cases: '声明的用例写在设计文件中，录制的用例由探索按覆盖选出；覆盖增量是相对谓词的其他用例新增的行数。',
  adequacy: '代码覆盖率是谓词执行到的函数中被执行的行；分支覆盖率是走到的步骤与分支；错误覆盖率是实际抛出的声明错误。',
  history: '该用例在各次运行中的结果，最新的在下。',
  covers: '该用例执行到的函数及其所在的行。',
  details: '未画出的细节函数：它们不落实设计节点，也没有检查或生成的绑定。',
  fields: '设计中写出的字段与落实它的代码类型逐项比较；代码中多出的字段算细节，不列出。',
  calls: '调用方与被调用方；只经过细节函数的调用画成虚线，标出经过的细节函数个数。',
  code: '落实它的代码元素的源码。',
  diagnostics: '检查快照得到的诊断；每一条都是错误。',
  compare: '与所选提交的快照比较，列出新增、删除和修改的节点与边。',
  architecture: '核心组件展开，显示其中的类型、流程、性质与错误；支撑组件折叠；工具组件不画出。边是实际的依赖，红色虚线为设计中没有声明的依赖。',
  ddd: '带限界上下文标签的组件，其中带领域标签的类型，以及它们之间的引用、继承与实现。',
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
