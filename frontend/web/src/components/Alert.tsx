import { createRoot } from "react-dom/client";
import { flushSync } from "react-dom";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

type AlertStyle = "default" | "destructive" | "danger";

interface Props {
  title: string;
  content: string;
  style?: AlertStyle;
  closeBtnText?: string;
  confirmBtnText?: string;
  onClose?: () => void;
  onConfirm?: () => void;
}

const defaultProps: Partial<Props> = {
  style: "default",
  closeBtnText: "Close",
  confirmBtnText: "Confirm",
  onClose: () => null,
  onConfirm: () => null,
};

const Alert: React.FC<Props> = (props: Props) => {
  const { title, content, closeBtnText, confirmBtnText, onClose, onConfirm, style } = {
    ...defaultProps,
    ...props,
  };

  const handleCloseBtnClick = () => {
    if (onClose) {
      onClose();
    }
  };

  const handleConfirmBtnClick = async () => {
    if (onConfirm) {
      onConfirm();
    }
  };

  return (
    <AlertDialog open={true}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{content}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={handleCloseBtnClick}>{closeBtnText}</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirmBtnClick}
            className={
              style === "destructive" || style === "danger" ? "bg-destructive text-destructive-foreground hover:bg-destructive/90" : ""
            }
          >
            {confirmBtnText}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
};

export const showCommonDialog = (props: Props) => {
  const tempDiv = document.createElement("div");
  const dialog = createRoot(tempDiv);
  document.body.append(tempDiv);

  let destroyed = false;
  const destroy = () => {
    if (destroyed) return;
    destroyed = true;
    window.removeEventListener("pagehide", destroy);
    dialog.unmount();
    tempDiv.remove();
  };

  window.addEventListener("pagehide", destroy, { once: true });

  const onClose = () => {
    if (props.onClose) {
      props.onClose();
    }
    destroy();
  };

  const onConfirm = () => {
    if (props.onConfirm) {
      props.onConfirm();
    }
    destroy();
  };

  flushSync(() => {
    dialog.render(<Alert {...props} onClose={onClose} onConfirm={onConfirm} />);
  });
};

export default Alert;
