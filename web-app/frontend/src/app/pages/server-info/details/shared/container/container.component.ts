import { Component, Input } from '@angular/core';
import { V1Container } from '@kubernetes/client-node';
import { ListEntry, ChipDescriptor } from 'kubeflow';

@Component({
  selector: 'app-container-details',
  templateUrl: './container.component.html',
})
export class ContainerComponent {
  public cmd: string;
  public env: ChipDescriptor[];

  @Input()
  set container(c: V1Container) {
    this.containerPrv = c;

    this.env = this.generateEnv(c);
    this.cmd = this.generateCmd(c);
  }
  get container() {
    return this.containerPrv;
  }

  private containerPrv: V1Container;

  private generateEnv(c: V1Container): ChipDescriptor[] {
    const chips = [];
    if (!c.env) {
      return null;
    }

    for (const envVar of c.env) {
      chips.push({
        value: `${envVar.name}: ${envVar.value}`,
        color: 'primary',
        tooltip: envVar.value,
      });
    }

    return chips;
  }

  private generateCmd(c: V1Container): string {
    if (!c.command) {
      return null;
    }

    let cmd = c.command.join(' ');
    if (!c.args) {
      return cmd;
    }

    cmd += [cmd, ...c.args].join(' ');
    return cmd;
  }
}
