import { useCallback, useEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";

export type HoverTooltip = {
  text: string;
  x: number;
  y: number;
  visible: boolean;
  flip: boolean;
  token: number;
};

function resolveTooltipText(target: HTMLElement | null) {
  if (!target) return "";
  const button = target.closest("button") as HTMLElement | null;
  if (!button) return "";
  if (button.matches("button:disabled")) return "";
  const explicit = button.getAttribute("data-tooltip") ?? button.getAttribute("aria-label");
  if (explicit?.trim()) return explicit.trim();
  const text = (button.textContent ?? "").replace(/\s+/g, " ").trim();
  if (!text || (text.length <= 2 && !/[A-Za-z0-9]/.test(text))) return "";
  return text;
}

export function useTooltip() {
  const [hoverTooltip, setHoverTooltip] = useState<HoverTooltip | null>(null);
  const tooltipOpenTimerRef = useRef<number | null>(null);
  const tooltipCloseTimerRef = useRef<number | null>(null);
  const tooltipTargetRef = useRef<HTMLElement | null>(null);
  const tooltipTokenRef = useRef(0);
  const tooltipRef = useRef<HTMLDivElement | null>(null);

  const clearTooltipTimers = useCallback(() => {
    if (tooltipOpenTimerRef.current !== null) {
      window.clearTimeout(tooltipOpenTimerRef.current);
      tooltipOpenTimerRef.current = null;
    }
    if (tooltipCloseTimerRef.current !== null) {
      window.clearTimeout(tooltipCloseTimerRef.current);
      tooltipCloseTimerRef.current = null;
    }
  }, []);

  const hideTooltip = useCallback(() => {
    clearTooltipTimers();
    tooltipTargetRef.current = null;
    setHoverTooltip((current) => {
      if (!current) return null;
      return { ...current, visible: false };
    });
    tooltipCloseTimerRef.current = window.setTimeout(() => {
      setHoverTooltip((current) => (current && !current.visible ? null : current));
    }, 160);
  }, [clearTooltipTimers]);

  const handleTooltipMove = useCallback(
    (event: ReactMouseEvent<HTMLElement>) => {
      const target = event.target as HTMLElement | null;
      const button = target?.closest("button") as HTMLElement | null;
      const text = resolveTooltipText(target);
      if (!button || !text) {
        if (tooltipTargetRef.current || hoverTooltip) {
          hideTooltip();
        }
        return;
      }

      if (tooltipTargetRef.current !== button) {
        clearTooltipTimers();
        tooltipTargetRef.current = button;
        const token = ++tooltipTokenRef.current;
        setHoverTooltip({ text, x: event.clientX, y: event.clientY, visible: false, flip: false, token });
        tooltipOpenTimerRef.current = window.setTimeout(() => {
          setHoverTooltip((current) => (current && current.token === token ? { ...current, visible: true } : current));
        }, 1000);
        return;
      }

      setHoverTooltip((current) =>
        current && current.token === tooltipTokenRef.current
          ? { ...current, text, x: event.clientX, y: event.clientY }
          : current,
      );
    },
    [clearTooltipTimers, hideTooltip, hoverTooltip],
  );

  useEffect(() => {
    if (!hoverTooltip?.visible || !tooltipRef.current) return;

    const updatePlacement = () => {
      const tooltip = tooltipRef.current;
      if (!tooltip) return;
      const width = tooltip.getBoundingClientRect().width;
      if (!width) return;
      setHoverTooltip((current) => {
        if (!current || !current.visible || current.token !== hoverTooltip.token) return current;
        const shouldFlip = current.x + width + 20 > window.innerWidth && current.x - width - 20 >= 8;
        if (current.flip === shouldFlip) return current;
        return { ...current, flip: shouldFlip };
      });
    };

    const raf = window.requestAnimationFrame(updatePlacement);
    window.addEventListener("resize", updatePlacement);
    return () => {
      window.cancelAnimationFrame(raf);
      window.removeEventListener("resize", updatePlacement);
    };
  }, [
    hoverTooltip?.visible,
    hoverTooltip?.token,
    hoverTooltip?.x,
    hoverTooltip?.y,
    hoverTooltip?.text,
    hoverTooltip?.flip,
  ]);

  useEffect(() => {
    return () => {
      clearTooltipTimers();
    };
  }, [clearTooltipTimers]);

  return {
    hoverTooltip,
    tooltipRef,
    handleTooltipMove,
    handleTooltipLeave: hideTooltip,
  };
}
