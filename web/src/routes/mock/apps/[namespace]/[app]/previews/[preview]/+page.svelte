<script lang="ts">
  import {
    Alert,
    Badge,
    Card,
    CardBody,
    CardHeader,
    CardTitle,
    ContextMenuButton,
    ControlBar,
    ControlBarContent,
    ControlBarDescription,
    ControlBarHeader,
    ControlBarOverflow,
    ControlBarTitle,
    Link,
    Table,
    TableBody,
    TableCell,
    TableHeader,
    TableHeading,
    TableRow,
    Text,
  } from '@immich/ui';
  import { mdiRobotOutline } from '@mdi/js';
  import PhaseBadge from '#lib/components/PhaseBadge.svelte';
  import { conditionColor, findCondition } from '#lib/kube/conditions.ts';
  import { createdByAnnotation, ownerLabel, owners, parameterRows } from '#lib/kube/preview.ts';
  import { getPreviewActions, menuItems } from '#lib/mock/actions.ts';
  import LinkButtons from '#lib/mock/components/LinkButtons.svelte';
  import PageLayout from '#lib/mock/components/PageLayout.svelte';
  import Time from '#lib/mock/components/Time.svelte';
  import { findApp, findPreview, isFailing, managedBy, restoreLabel, source } from '#lib/mock/model.ts';
  import { Route } from '#lib/mock/routes.ts';
  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const preview = $derived(findPreview(params.namespace, params.preview));
  const app = $derived(findApp(params.namespace, params.app));
  const breadcrumbs = $derived([
    { title: params.namespace },
    { title: params.app, href: Route.app(params.namespace, params.app) },
    { title: params.preview },
  ]);
</script>

<PageLayout {breadcrumbs}>
  {#if preview}
    {@const ready = findCondition(preview, 'Ready')}
    {@const failing = isFailing(preview)}
    {@const manager = managedBy(preview)}
    {@const from = source(preview)}
    {@const createdBy = preview.metadata?.annotations?.[createdByAnnotation]}
    {@const { Edit, Reset, Snapshot, Delete } = getPreviewActions(preview)}
    {@const items = menuItems([Edit, Reset, Snapshot], [Delete])}

    <ControlBar variant="ghost" static closeOnEsc={false} class="p-0">
      <ControlBarHeader>
        <ControlBarTitle tag="h1" size="large" fontWeight="bold">{params.preview}</ControlBarTitle>
        <ControlBarDescription>
          Created <Time timestamp={preview.metadata?.creationTimestamp} />{createdBy ? ` by ${createdBy}` : ''}
        </ControlBarDescription>
      </ControlBarHeader>
      <ControlBarContent>
        <PhaseBadge phase={preview.status?.phase} />
      </ControlBarContent>
      <ControlBarOverflow class="flex-wrap">
        <LinkButtons links={preview.status?.links} size="medium" />
        <ContextMenuButton aria-label="Actions" {items} />
      </ControlBarOverflow>
    </ControlBar>

    {#if failing && ready}
      <Alert color="danger" title={ready.reason}>{ready.message}</Alert>
    {:else if ready && ready.status !== 'True'}
      <Alert color="info" title={ready.message} />
    {/if}

    {#if manager}
      <Alert
        color="secondary"
        icon={mdiRobotOutline}
        title={`Managed by ${manager.tool} ${manager.kind} ${manager.namespace}/${manager.name}`}
      >
        Edit and delete it there. Reset and snapshot work here.
      </Alert>
    {/if}

    <div class="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader><CardTitle>Parameters</CardTitle></CardHeader>
        <CardBody>
          <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
            {#each parameterRows(preview, app) as row (row.name)}
              <dt class="font-medium">{row.title ?? row.name}</dt>
              <dd class="flex items-center gap-2">
                <span class="font-mono">{row.value}</span>
                {#if row.isDefault}
                  <Text size="tiny" color="muted">default</Text>
                {/if}
              </dd>
            {:else}
              <Text size="small" color="muted">No parameters</Text>
            {/each}
          </dl>
        </CardBody>
      </Card>

      <Card>
        <CardHeader><CardTitle>Details</CardTitle></CardHeader>
        <CardBody>
          <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
            <dt class="font-medium">Source</dt>
            <dd>
              {#if from}
                <Link href={from.url} target="_blank" rel="noreferrer">{from.title}</Link>
              {:else}
                <Text size="small" color="muted">Unknown</Text>
              {/if}
            </dd>
            <dt class="font-medium">Started from</dt>
            <dd>
              {#if preview.status?.restoredFrom}
                <Link href={Route.snapshots(params.namespace, params.app)}>{preview.status.restoredFrom.name}</Link>
              {:else}
                Empty
              {/if}
              {#if preview.spec.restoreFrom?.snapshotSelector}
                <Text size="tiny" color="muted">{restoreLabel(preview)} on reset</Text>
              {/if}
            </dd>
            <dt class="font-medium">Owners</dt>
            <dd>{owners(preview).map(ownerLabel).join(', ') || '–'}</dd>
            <dt class="font-medium">Namespace</dt>
            <dd><span class="font-mono">{preview.status?.namespace}</span></dd>
          </dl>
        </CardBody>
      </Card>
    </div>

    <Card expandable expanded={failing}>
      <CardHeader><CardTitle>Conditions</CardTitle></CardHeader>
      <CardBody>
        <Table spacing="small" size="small">
          <TableHeader>
            <TableHeading class="flex-[2] px-1">Type</TableHeading>
            <TableHeading class="flex-[2] px-1">Status</TableHeading>
            <TableHeading class="hidden flex-[3] px-1 md:block">Reason</TableHeading>
            <TableHeading class="flex-[5] px-1">Message</TableHeading>
            <TableHeading class="hidden flex-[2] px-1 md:block">Age</TableHeading>
          </TableHeader>
          <TableBody>
            {#each preview.status?.conditions ?? [] as condition (condition.type)}
              <TableRow>
                <TableCell class="flex-[2] px-1">{condition.type}</TableCell>
                <TableCell class="flex-[2] px-1">
                  <Badge size="small" color={conditionColor(condition)}>{condition.status}</Badge>
                </TableCell>
                <TableCell class="hidden flex-[3] break-words md:block">{condition.reason}</TableCell>
                <TableCell class="flex-[5] break-words">{condition.message}</TableCell>
                <TableCell class="hidden flex-[2] px-1 md:block"
                  ><Time timestamp={condition.lastTransitionTime} /></TableCell
                >
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      </CardBody>
    </Card>

    <Card expandable expanded={failing}>
      <CardHeader><CardTitle>Inventory</CardTitle></CardHeader>
      <CardBody>
        <Table spacing="small" size="small">
          <TableHeader>
            <TableHeading class="flex-[5] px-1">Kind</TableHeading>
            <TableHeading class="flex-[5] px-1">Name</TableHeading>
            <TableHeading class="flex-[2] px-1"></TableHeading>
          </TableHeader>
          <TableBody>
            {#each preview.status?.inventory ?? [] as entry (`${entry.apiGroup}/${entry.kind}/${entry.name}`)}
              <TableRow>
                <TableCell class="flex-[5] px-1"
                  >{entry.apiGroup ? `${entry.kind}.${entry.apiGroup}` : entry.kind}</TableCell
                >
                <TableCell class="flex-[5] px-1">{entry.name}</TableCell>
                <TableCell class="flex-[2] px-1">
                  {#if entry.state}
                    <Badge size="small" color="info">State</Badge>
                  {/if}
                </TableCell>
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      </CardBody>
    </Card>
  {:else}
    <Text color="muted">Preview {params.preview} not found</Text>
  {/if}
</PageLayout>
