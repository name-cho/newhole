import React from "react";
import classNames from "classnames";
import { useDispatch, useSelector } from "react-redux";
import { send } from "@giantmachines/redux-websocket";
import { isNewholeHostnameValid } from "../../features/validator/validators";
import {
  MessageTypeRequestHostnameForget,
  MessageTypeRequestHostnameHistory,
} from "../../constants/websocket";

interface HostnameRecord {
  hostname: string;
  domain: string;
  firstUsed: string;
  lastUsed: string;
  uses: number;
  exposeMode: string;
}

interface BasicAuthSettingsProps {
  usingValue: boolean;
  usingChangeCallback: Function;
  hostnameValue: string;
  hostnameChangeCallback: Function;
}

const HostnameSettings = (props: BasicAuthSettingsProps): JSX.Element => {
  const dispatch = useDispatch();
  const hostnames: HostnameRecord[] = useSelector(
    (state: any) => state.config.hostnames || []
  );
  const usingCustomHostname = props.usingValue;
  const setUsingCustomHostname = props.usingChangeCallback;
  const customHostname = props.hostnameValue;
  const setCustomHostname = props.hostnameChangeCallback;

  const isHostnameValid = (): boolean => {
    return isNewholeHostnameValid(customHostname);
  };

  return (
    <div>
      <div className="field">
        <div className="control">
          <label className="checkbox">
            <input
              type="checkbox"
              onChange={(e) => {
                setUsingCustomHostname(!usingCustomHostname);
              }}
            />{" "}
            I want to use custom hostname
          </label>
        </div>
      </div>
      {usingCustomHostname ? (
        <div className="field">
          <label className="label">Custom hostname</label>
          <div className="control has-icons-left has-icons-right">
            <input
              className={classNames({
                input: true,
                "is-success": isHostnameValid(),
                "is-danger": !isHostnameValid(),
              })}
              type="text"
              placeholder="Hostname to expose tunnel on"
              value={customHostname}
              onChange={(e) => setCustomHostname(e.target.value.toLowerCase())}
            />
            <span className="icon is-small is-left">
              <i className="fas fa-signature"></i>
            </span>
            <span className="icon is-small is-right">
              <i
                className={classNames({
                  fas: true,
                  "fa-check": isHostnameValid(),
                  "fa-exclamation-triangle": !isHostnameValid(),
                })}
              ></i>
            </span>
          </div>
          {isHostnameValid() ? (
            <p className="help is-success">Hostname is valid</p>
          ) : (
            <p className="help is-danger">Hostname is invalid</p>
          )}
        </div>
      ) : null}
      {usingCustomHostname && hostnames.length > 0 ? (
        <div className="field">
          <label className="label">Previously used hostnames</label>
          <div className="control">
            <div className="select is-fullwidth">
              <select
                defaultValue=""
                onChange={(e) => {
                  const hostname = e.target.value;
                  if (hostname === "") return;
                  setCustomHostname(hostname);
                }}
              >
                <option value="">Select a hostname used before</option>
                {hostnames.map((record) => (
                  <option key={record.hostname} value={record.hostname}>
                    {record.hostname} ({record.uses} uses, last{" "}
                    {record.lastUsed ? record.lastUsed.substring(0, 10) : "unknown"})
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="buttons are-small mt-2">
            <button
              className="button is-light"
              type="button"
              onClick={() =>
                dispatch(send({ type: MessageTypeRequestHostnameHistory }))
              }
            >
              Refresh
            </button>
            {hostnames.some(
              (record) => record.hostname === customHostname && customHostname !== ""
            ) ? (
              <button
                className="button is-danger is-light"
                type="button"
                onClick={() =>
                  dispatch(
                    send({
                      type: MessageTypeRequestHostnameForget,
                      payload: { hostname: customHostname },
                    })
                  )
                }
              >
                Forget {customHostname}
              </button>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
};

export default HostnameSettings;
