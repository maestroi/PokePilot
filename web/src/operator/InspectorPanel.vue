<script lang="ts">
import { defineComponent, h } from 'vue'
import InspectorPanelInner from './InspectorPanelInner.vue'

export default defineComponent({
  name: 'InspectorPanel',
  props: {
    runId: { type: String, required: true }
  },
  setup(props) {
    return () => h(InspectorPanelInner as any, { runID: props.runId })
  }
})
</script>
