import { useQuery, useSuspenseQuery } from '@tanstack/react-query';
import createClient from 'openapi-fetch';
import { ApiError } from './errors';
import type { paths } from './schema';
import type { C4Level, ClassLayer, EdgeKind, Problem, ViewKind } from './types';

/** Where the interface is served: `@server("/api")` in api/main.tsp. */
export const API_ROOT = '/api';

export const api = createClient<paths>({ baseUrl: API_ROOT });

/** The body of an answer; a refusal becomes a thrown ApiError, which the pane's error boundary shows. */
export async function responseBody<T>(request: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const { data, error, response } = await request;
  if (data === undefined) {
    throw new ApiError(response.status, error as Problem);
  }
  return data;
}

export function useSnapshot() {
  return useSuspenseQuery({ queryKey: ['snapshot'], queryFn: () => responseBody(api.GET('/snapshot')) }).data;
}

export function useTree() {
  return useSuspenseQuery({ queryKey: ['tree'], queryFn: () => responseBody(api.GET('/tree')) }).data;
}

export function useDiagnostics() {
  return useSuspenseQuery({ queryKey: ['diagnostics'], queryFn: () => responseBody(api.GET('/diagnostics')) }).data;
}

/** What a view request names, as the address holds it. */
export interface ViewQuery {
  kind: ViewKind;
  input?: string;
  expand: string[];
  layer?: ClassLayer;
  level?: C4Level;
}

export function useView({ kind, input, expand, layer, level }: ViewQuery) {
  return useSuspenseQuery({
    queryKey: ['view', kind, input, expand, layer, level],
    queryFn: () => responseBody(api.GET('/views/{kind}', { params: { path: { kind }, query: { input, expand, layer, level } } })),
  }).data;
}

export function useEntity(id: string, base?: string) {
  return useSuspenseQuery({
    queryKey: ['entity', id, base],
    queryFn: () => responseBody(api.GET('/entity', { params: { query: { id, base } } })),
  }).data;
}

/** An entity that a control needs only when it exists, such as the input of the current view; undefined while it loads. */
export function useOptionalEntity(id: string | undefined) {
  return useQuery({
    queryKey: ['entity', id, undefined],
    queryFn: () => responseBody(api.GET('/entity', { params: { query: { id: id ?? '' } } })),
    enabled: id !== undefined,
  }).data;
}

export function useEdge(kind: EdgeKind, from: string, to: string) {
  return useSuspenseQuery({
    queryKey: ['edge', kind, from, to],
    queryFn: () => responseBody(api.GET('/edge', { params: { query: { kind, from, to } } })),
  }).data;
}

export function useNodes(ids: string[]) {
  return useSuspenseQuery({
    queryKey: ['nodes', ids],
    queryFn: () => (ids.length === 0 ? Promise.resolve([]) : responseBody(api.GET('/nodes', { params: { query: { id: ids } } }))),
  }).data;
}

/** The search box's matches; nothing is asked for an empty text. */
export function useSearch(text: string) {
  return useQuery({
    queryKey: ['search', text],
    queryFn: () => responseBody(api.GET('/search', { params: { query: { text, limit: SEARCH_LIMIT } } })),
    enabled: text.trim() !== '',
  });
}

/** How many matches the search box lists. */
export const SEARCH_LIMIT = 12;

export function useCode(path: string, from?: number, to?: number) {
  return useSuspenseQuery({
    queryKey: ['code', path, from, to],
    queryFn: () => responseBody(api.GET('/code', { params: { query: { path, from, to } } })),
  }).data;
}

export function useRevisions() {
  return useQuery({ queryKey: ['revisions'], queryFn: () => responseBody(api.GET('/revisions')) });
}

export function useComparison(base: string | undefined) {
  return useQuery({
    queryKey: ['compare', base],
    queryFn: () => responseBody(api.GET('/compare', { params: { query: { base: base ?? '' } } })),
    enabled: base !== undefined,
  });
}
