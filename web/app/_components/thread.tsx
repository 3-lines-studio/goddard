import { useRef, useState } from "react";
import {
  ArrowDownIcon,
  CheckIcon,
  ChevronRightIcon,
  CircleIcon,
  CopyIcon,
  DownloadIcon,
  FileIcon,
  FilePlusIcon,
  GlobeIcon,
  PaperclipIcon,
  PencilIcon,
  SearchIcon,
  SendIcon,
  SquareIcon,
  TerminalIcon,
  TriangleAlertIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment";
import { Bubble, BubbleContent } from "@/components/ui/bubble";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker";
import { Message, MessageContent } from "@/components/ui/message";
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller";

import { markdown } from "../_lib/markdown";
import { describeTool } from "../_lib/tool";
import type { Event, Upload } from "./types";

const TOOL_ICON: Record<string, LucideIcon> = {
  read: FileIcon,
  write: FilePlusIcon,
  edit: PencilIcon,
  bash: TerminalIcon,
  search: SearchIcon,
  fetch: DownloadIcon,
  browse: GlobeIcon,
};

export function Thread({
  lines,
  partial,
  workspace,
}: {
  lines: Event[];
  partial: string;
  workspace: string;
}) {
  return (
    <MessageScrollerProvider autoScroll>
      <MessageScroller>
        <MessageScrollerViewport>
          <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-3 p-4">
            {lines.map((line, index) => (
              <MessageScrollerItem key={index} messageId={String(index)}>
                <Row line={line} workspace={workspace} />
              </MessageScrollerItem>
            ))}
            {partial ? (
              <MessageScrollerItem messageId="partial" scrollAnchor>
                <Message align="start">
                  <MessageContent>
                    <Bubble variant="ghost" align="start">
                      <BubbleContent className="whitespace-pre-wrap">{partial}</BubbleContent>
                    </Bubble>
                  </MessageContent>
                </Message>
              </MessageScrollerItem>
            ) : null}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton>
          <ArrowDownIcon />
          <span className="sr-only">Ir al final</span>
        </MessageScrollerButton>
      </MessageScroller>
    </MessageScrollerProvider>
  );
}

function Row({ line, workspace }: { line: Event; workspace: string }) {
  switch (line.event) {
    case "user":
      return (
        <Message align="end">
          <MessageContent>
            <Bubble variant="secondary" align="end">
              <BubbleContent className="whitespace-pre-wrap">{line.text}</BubbleContent>
            </Bubble>
          </MessageContent>
        </Message>
      );
    case "assistant":
      return <Assistant text={line.text ?? ""} />;
    case "tool_start":
      return <Tool call={line} done={false} workspace={workspace} />;
    case "tool_result":
      return <Tool call={line} done workspace={workspace} />;
    case "file":
      return <File line={line} />;
    case "error":
      return (
        <Alert variant="destructive" data-error="">
          <AlertDescription>{line.message}</AlertDescription>
        </Alert>
      );
    case "stopped":
      return (
        <Marker>
          <MarkerIcon>
            <SquareIcon />
          </MarkerIcon>
          <MarkerContent>frenado</MarkerContent>
        </Marker>
      );
    default:
      return null;
  }
}

function Assistant({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  return (
    <Message align="start">
      <MessageContent>
        <Bubble variant="ghost" align="start">
          <BubbleContent>
            <div className="md text-sm" dangerouslySetInnerHTML={{ __html: markdown(text) }} />
          </BubbleContent>
        </Bubble>
      </MessageContent>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        data-copy=""
        aria-label={copied ? "copiado" : "copiar"}
        onClick={() => {
          void navigator.clipboard.writeText(text).then(() => setCopied(true));
        }}
        className="absolute end-1 top-0 opacity-0 group-hover/message:opacity-100"
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </Button>
    </Message>
  );
}

function Tool({ call, done, workspace }: { call: Event; done: boolean; workspace: string }) {
  const what = describeTool(call.name ?? "", call.args ?? "", workspace);
  const Glyph = TOOL_ICON[call.name ?? ""] ?? CircleIcon;
  return (
    <Collapsible className="group/tool rounded-lg border border-border">
      <CollapsibleTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            className="h-auto w-full justify-start gap-2 rounded-lg px-2 py-1.5 font-normal"
          />
        }
      >
        <Glyph data-icon="inline-start" className="text-muted-foreground" />
        <span className="shrink-0 font-medium">{call.name}</span>
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {what.dir ? `${what.dir} · ` : ""}
          {what.text}
        </span>
        {done ? <span className="shrink-0 text-xs text-muted-foreground">ok · {call.ms} ms</span> : null}
        <ChevronRightIcon className="shrink-0 text-muted-foreground transition-transform group-data-open/tool:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="overflow-x-auto border-t border-border px-2 py-1.5 text-xs text-muted-foreground">
          {done ? call.text : call.args}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function File({ line }: { line: Event }) {
  const href = `/api/uploads?id=${line.id}`;
  if ((line.mime ?? "").startsWith("image/")) {
    return (
      <Message align="start">
        <MessageContent>
          <Bubble variant="outline" align="start">
            <BubbleContent className="p-0">
              <a href={href} target="_blank" rel="noreferrer">
                <img src={href} alt={line.name ?? ""} className="max-h-72 w-auto" />
              </a>
            </BubbleContent>
          </Bubble>
          {line.caption ? <Marker>{line.caption}</Marker> : null}
        </MessageContent>
      </Message>
    );
  }
  return (
    <Attachment state="done" data-file={line.id}>
      <AttachmentMedia variant="icon">
        <FileIcon />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>{line.name}</AttachmentTitle>
        <AttachmentDescription>{line.caption || "archivo"}</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <AttachmentAction
          render={<a href={href} target="_blank" rel="noreferrer" aria-label={`Bajar ${line.name}`} />}
        >
          <DownloadIcon />
        </AttachmentAction>
      </AttachmentActions>
    </Attachment>
  );
}

export function Composer({
  open,
  text,
  busy,
  working,
  uploads,
  onText,
  onSend,
  onStop,
  onAttach,
  onDetach,
}: {
  open: string;
  text: string;
  busy: boolean;
  working: boolean;
  uploads: Upload[];
  onText: (text: string) => void;
  onSend: (event: React.FormEvent<HTMLFormElement>) => void;
  onStop: () => void;
  onAttach: (files: FileList | null) => void;
  onDetach: (id: string) => void;
}) {
  const files = useRef<HTMLInputElement>(null);
  return (
    <form onSubmit={onSend} className="border-t p-3">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-2">
        {uploads.length > 0 ? (
          <AttachmentGroup>
            {uploads.map((one) => (
              <Attachment key={one.id} size="xs" state="done" data-upload={one.id}>
                <AttachmentMedia variant="icon">
                  <FileIcon />
                </AttachmentMedia>
                <AttachmentContent>
                  <AttachmentTitle>{one.name}</AttachmentTitle>
                </AttachmentContent>
                <AttachmentActions>
                  <AttachmentAction
                    data-detach={one.id}
                    aria-label={`Sacar ${one.name}`}
                    onClick={() => onDetach(one.id)}
                  >
                    <XIcon />
                  </AttachmentAction>
                </AttachmentActions>
              </Attachment>
            ))}
          </AttachmentGroup>
        ) : null}
        <InputGroup>
          <InputGroupTextarea
            data-composer=""
            value={text}
            rows={2}
            placeholder={open ? "Escribí un mensaje…" : "Elegí una conversación primero"}
            disabled={!open}
            onChange={(event) => onText(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
          />
          <InputGroupAddon align="block-end">
            <input
              ref={files}
              type="file"
              multiple
              data-files=""
              className="sr-only"
              onChange={(event) => onAttach(event.target.files)}
            />
            <InputGroupButton
              data-attach=""
              onClick={() => files.current?.click()}
              disabled={!open}
              aria-label="Adjuntar"
            >
              <PaperclipIcon />
            </InputGroupButton>
            <span className="ms-auto flex items-center gap-1">
              {working ? (
                <InputGroupButton variant="outline" data-stop="" onClick={onStop}>
                  detener
                </InputGroupButton>
              ) : null}
              <InputGroupButton
                type="submit"
                variant="default"
                size="sm"
                data-send=""
                disabled={!open || busy || working || (!text.trim() && uploads.length === 0)}
              >
                <SendIcon data-icon="inline-start" />
                Enviar
              </InputGroupButton>
            </span>
          </InputGroupAddon>
        </InputGroup>
      </div>
    </form>
  );
}
