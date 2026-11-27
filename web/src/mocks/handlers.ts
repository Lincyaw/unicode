import { createOpenApiHttp } from 'openapi-msw';
import { viewKindValues, type paths } from '../api/schema';
import { comparisonWith, resolveRevision } from './compare';
import { entityOf } from './entities';
import { excerptOf, isServedFile } from './excerpts';
import { has } from './graph';
import { diagnostics, edgeEntity, impact, nodesById, nodeView, search, slice, snapshot, tree } from './queries';
import { REVISIONS } from './shopChecks';
import type { C4Level, ClassLayer, EdgeKind, NodeKind, Problem, ViewKind } from './schemaTypes';
import { NotApplicable, NotFound } from './viewKit';
import { viewOf } from './views';

/*
 * The mock of `unicode serve`: every operation of api/openapi.yaml, answered from the shop's snapshot. The handlers
 * are typed by the interface, so a path, parameter, status or body it does not declare fails to compile.
 */

const http = createOpenApiHttp<paths>({ baseUrl: '/api' });

/** The request's query parameters, a repeated one as all its values. */
function queryOf(request: Request): URLSearchParams {
  return new URL(request.url).searchParams;
}

function notFound(message: string): Problem {
  return { code: 'serve.not-found', message, fix: 'take an identity, an edge or a path from the snapshot; rebuild with unicode build if it was just added' };
}

function notApplicable(message: string): Problem {
  return { code: 'serve.not-applicable', message, fix: 'open the view with an input it takes: docs/design.md §7.1 lists each view\'s input' };
}

function badRevision(rev: string): Problem {
  return { code: 'serve.bad-revision', message: `git cannot read the revision ${rev}`, fix: 'name a commit of the branch, such as one /api/revisions lists, or main~1' };
}

function badParameter(message: string): Problem {
  return { code: 'serve.bad-parameter', message, fix: 'give every required parameter, with a value of its type: api/openapi.yaml lists them' };
}

function badPath(path: string): Problem {
  return { code: 'serve.bad-path', message: `${path} is not a design file, a corpus file or a code file of the snapshot`, fix: 'read a file the snapshot describes: a node\'s origin gives one' };
}

/** A not-found identity the request named. */
function missing(ids: string[]): string | undefined {
  return ids.find(id => !has(id));
}

export const handlers = [
  http.get('/snapshot', ({ response }) => response(200).json(snapshot())),

  http.get('/tree', ({ response }) => response(200).json(tree())),

  http.get('/revisions', ({ request, response }) => {
    const limit = Number(queryOf(request).get('limit') ?? 20);
    return response(200).json(REVISIONS.slice(0, limit));
  }),

  http.get('/views/{kind}', ({ params, request, response }) => {
    const query = queryOf(request);
    if (!viewKindValues.includes(params.kind as ViewKind)) {
      return response(400).json(badParameter(`there is no view ${params.kind}`));
    }
    try {
      const view = viewOf(params.kind as ViewKind, {
        ...(query.get('input') !== null ? { input: query.get('input') ?? '' } : {}),
        expand: query.getAll('expand'),
        ...(query.get('layer') !== null ? { layer: query.get('layer') as ClassLayer } : {}),
        ...(query.get('level') !== null ? { level: query.get('level') as C4Level } : {}),
      });
      return response(200).json(view);
    } catch (error) {
      if (error instanceof NotFound) {
        return response(404).json(notFound(error.message));
      }
      if (error instanceof NotApplicable) {
        return response(400).json(notApplicable(error.message));
      }
      throw error;
    }
  }),

  http.get('/entity', ({ request, response }) => {
    const query = queryOf(request);
    const id = query.get('id') ?? '';
    if (!has(id)) {
      return response(404).json(notFound(`the snapshot has no node ${id}`));
    }
    const rev = query.get('base');
    if (rev === null) {
      return response(200).json(entityOf(id));
    }
    const base = resolveRevision(rev);
    return base ? response(200).json(entityOf(id, base)) : response(400).json(badRevision(rev));
  }),

  http.get('/edge', ({ request, response }) => {
    const query = queryOf(request);
    try {
      return response(200).json(edgeEntity(query.get('kind') as EdgeKind, query.get('from') ?? '', query.get('to') ?? ''));
    } catch (error) {
      if (error instanceof NotFound) {
        return response(404).json(notFound(error.message));
      }
      throw error;
    }
  }),

  http.get('/nodes', ({ request, response }) => response(200).json(nodesById(queryOf(request).getAll('id')))),

  http.get('/node', ({ request, response }) => {
    const id = queryOf(request).get('id') ?? '';
    return has(id) ? response(200).json(nodeView(id)) : response(404).json(notFound(`the snapshot has no node ${id}`));
  }),

  http.get('/search', ({ request, response }) => {
    const query = queryOf(request);
    const kind = query.get('kind');
    return response(200).json(search(query.get('text') ?? '', kind === null ? undefined : (kind as NodeKind), Number(query.get('limit') ?? 50)));
  }),

  http.get('/slice', ({ request, response }) => {
    const query = queryOf(request);
    const ids = query.getAll('id');
    const absent = missing(ids);
    if (absent !== undefined) {
      return response(404).json(notFound(`the snapshot has no node ${absent}`));
    }
    return response(200).json(slice(ids, query.getAll('expand'), query.get('withCode') === 'true'));
  }),

  http.get('/impact', ({ request, response }) => {
    const ids = queryOf(request).getAll('id');
    const absent = missing(ids);
    return absent === undefined ? response(200).json(impact(ids)) : response(404).json(notFound(`the snapshot has no node ${absent}`));
  }),

  http.get('/diagnostics', ({ response }) => response(200).json(diagnostics())),

  http.get('/code', ({ request, response }) => {
    const query = queryOf(request);
    const path = query.get('path') ?? '';
    if (!isServedFile(path)) {
      return response(400).json(badPath(path));
    }
    const [from, to] = [query.get('from'), query.get('to')];
    return response(200).json(excerptOf(path, from === null ? undefined : Number(from), to === null ? undefined : Number(to)));
  }),

  http.get('/compare', ({ request, response }) => {
    const rev = queryOf(request).get('base') ?? '';
    const base = resolveRevision(rev);
    return base ? response(200).json(comparisonWith(base)) : response(400).json(badRevision(rev));
  }),
];
