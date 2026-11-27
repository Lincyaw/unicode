import { createBrowserRouter, Navigate } from 'react-router';
import { DEFAULT_VIEW, ENTITY_PATH, ROOT_PATH, VIEW_PATH, viewPath } from '../routes/address';
import { EntityPage } from './EntityPage';
import { ViewPage } from './ViewPage';

/** The viewer's pages: a view, an entity page, and the root, which opens the default view. */
export const router = createBrowserRouter([
  { path: ROOT_PATH, element: <Navigate to={viewPath({ kind: DEFAULT_VIEW, expand: [] })} replace /> },
  { path: VIEW_PATH, element: <ViewPage /> },
  { path: ENTITY_PATH, element: <EntityPage /> },
  { path: '*', element: <Navigate to={ROOT_PATH} replace /> },
]);
