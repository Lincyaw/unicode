import { createBrowserRouter, Navigate } from 'react-router';
import { DEFAULT_VIEW, ENTITY_PATH, ROOT_PATH, VIEW_PATH, viewPath } from '../routes/address';
import { AddressProblem } from './AddressProblem';
import { EntityPage } from './EntityPage';
import { ViewPage } from './ViewPage';

/** The viewer's pages: a view, an entity page, and the root, which opens the default view; an address none can read shows why. */
export const router = createBrowserRouter([
  { path: ROOT_PATH, element: <Navigate to={viewPath({ kind: DEFAULT_VIEW, expand: [] })} replace /> },
  { path: VIEW_PATH, element: <ViewPage />, errorElement: <AddressProblem /> },
  { path: ENTITY_PATH, element: <EntityPage />, errorElement: <AddressProblem /> },
  { path: '*', element: <Navigate to={ROOT_PATH} replace /> },
]);
