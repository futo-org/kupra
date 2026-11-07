<script lang="ts">
  import { Alert } from '@immich/ui';
  import { page } from '$app/state';
</script>

<Alert color="danger" title={`Error ${page.status}`}>{page.error?.message}</Alert>
