import { useView } from '../api/client';
import type { View } from '../api/types';
import { DrawingKey } from '../components/canvas/panZoom';
import { SwitchLinks } from '../components/shell/SwitchLinks';
import { ActivityView } from '../components/views/ActivityView';
import { ArchitectureView } from '../components/views/ArchitectureView';
import { C4View } from '../components/views/C4View';
import { CallsView } from '../components/views/CallsView';
import { ClassView } from '../components/views/ClassView';
import { CodeView } from '../components/views/CodeView';
import { DddView } from '../components/views/DddView';
import { EventsView } from '../components/views/EventsView';
import { PropertyView } from '../components/views/PropertyView';
import { SequenceView } from '../components/views/SequenceView';
import { viewPath, type ViewAddress } from '../routes/address';
import { C4_LEVELS, CLASS_LAYERS, CONTROLS } from '../terms';

/**
 * The drawing of the view the address names, drawn afresh for each input, layer and level, so that it opens where the
 * reader left it this session, else at its own scale; an expansion or a selection redraws it in place, keeping the
 * reader's zoom and pan.
 */
export function ViewDrawing({ address }: { address: ViewAddress }) {
  const view = useView(address);
  const key = [view.kind, view.input.id, view.layer, view.level].join(' ');
  return <DrawingKey key={key} value={key}><Drawing view={view} address={address} /></DrawingKey>;
}

/** The drawing of a view; the class view's layer and the C4 view's level switch in place. */
function Drawing({ view, address }: { view: View; address: ViewAddress }) {
  switch (view.kind) {
    case 'architecture':
      return <ArchitectureView view={view} />;
    case 'ddd':
      return <DddView view={view} />;
    case 'class':
      return (
        <ClassView
          view={view}
          toolbar={<SwitchLinks label={CONTROLS.layer} current={view.layer ?? 'design'} items={(['design', 'code'] as const).map(layer => ({ key: layer, label: CLASS_LAYERS[layer], href: viewPath({ ...address, layer, select: undefined }) }))} />}
        />
      );
    case 'sequence':
      return <SequenceView view={view} />;
    case 'activity':
      return <ActivityView view={view} />;
    case 'events':
      return <EventsView view={view} />;
    case 'calls':
      return <CallsView view={view} />;
    case 'property':
      return <PropertyView view={view} />;
    case 'code':
      return <CodeView view={view} />;
    case 'c4':
      return (
        <C4View
          view={view}
          toolbar={<SwitchLinks label={CONTROLS.level} current={view.level ?? 'container'} items={(['context', 'container'] as const).map(level => ({ key: level, label: C4_LEVELS[level], href: viewPath({ ...address, level, select: undefined }) }))} />}
        />
      );
  }
}
