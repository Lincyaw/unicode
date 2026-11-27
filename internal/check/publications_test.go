package check

import (
	"github.com/lincyaw/unicode/internal/graph"
)

type publication struct {
	function, event string
	home            *comp
}

func (w *world) publicationSample() *publication {
	if len(w.publications) > 0 {
		return choose(w, w.publications)
	}
	home := w.comps[1]
	event := w.designType(home, home.id+".Published", "event", home.pkg+".Published", home.file, nil)
	for _, method := range []bool{false, true} {
		parent, id, form := home.file, home.pkg+".Publish", "func"
		if method {
			parent = w.typeNamed(objectOf(home)).code
			id, form = parent+".Publish", "method"
		}
		function := w.function(parent, id, "publish", "(event: "+event.id+")", form)
		w.declared(graph.EdgeKindPublishes, function, event.id)
		w.publications = append(w.publications, &publication{function, event.id, home})
	}
	return choose(w, w.publications)
}
